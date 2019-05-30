package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/spf13/cobra"
	coreapi "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/ipset"
	"github.com/weaveworks/weave/npc"
	"github.com/weaveworks/weave/npc/metrics"
	"github.com/weaveworks/weave/npc/ulogd"
)

var (
	version        = "unreleased"
	metricsAddr    string
	logLevel       string
	allowMcast     bool
	nodeName       string
	maxList        int
	bridgePortName string
)

func handleError(err error) { common.CheckFatal(err) }

func makeController(getter cache.Getter, resource string,
	objType runtime.Object, handlers cache.ResourceEventHandlerFuncs) cache.Controller {
	listWatch := cache.NewListWatchFromClient(getter, resource, "", fields.Everything())
	_, controller := cache.NewInformer(listWatch, objType, 0, handlers)
	return controller
}

func resetIPTables(ipt *iptables.IPTables) error {
	// Flush chains first so there are no refs to extant ipsets
	if err := ipt.ClearChain(npc.TableFilter, npc.IngressChain); err != nil {
		return err
	}

	if err := ipt.ClearChain(npc.TableFilter, npc.DefaultChain); err != nil {
		return err
	}

	if err := ipt.ClearChain(npc.TableFilter, npc.MainChain); err != nil {
		return err
	}

	if err := ipt.ClearChain(npc.TableFilter, npc.EgressMarkChain); err != nil {
		return err
	}

	if err := ipt.ClearChain(npc.TableFilter, npc.EgressCustomChain); err != nil {
		return err
	}

	if err := ipt.ClearChain(npc.TableFilter, npc.EgressDefaultChain); err != nil {
		return err
	}

	// We do not clear npc.EgressChain here because otherwise, in the case of restarting
	// weave-npc process, all egress traffic is allowed for a short period of time.
	// The chain is created in createBaseRules.

	return nil
}

func resetIPSets(ips ipset.Interface) error {
	// Remove ipsets prefixed `weave-` only.

	sets, err := ips.List(npc.IpsetNamePrefix)
	if err != nil {
		common.Log.Errorf("Failed to retrieve list of ipsets")
		return err
	}

	common.Log.Debugf("Got list of ipsets: %v", sets)

	// Must remove references to ipsets by other ipsets before they're destroyed
	for _, s := range sets {
		common.Log.Debugf("Flushing ipset '%s'", string(s))
		if err := ips.Flush(s); err != nil {
			common.Log.Errorf("Failed to flush ipset '%s'", string(s))
			return err
		}
	}

	for _, s := range sets {
		// LocalIPset might be used by WEAVE-NPC-EGRESS chain which we do not
		// flush, so we cannot destroy it.
		if s == npc.LocalIpset {
			continue
		}
		common.Log.Debugf("Destroying ipset '%s'", string(s))
		if err := ips.Destroy(s); err != nil {
			common.Log.Errorf("Failed to destroy ipset '%s'", string(s))
			return err
		}
	}

	return nil
}

func createBaseRules(ipt *iptables.IPTables, ips ipset.Interface) error {
	// Configure main chain static rules
	if err := ipt.Append(npc.TableFilter, npc.MainChain,
		"-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return err
	}

	if allowMcast {
		if err := ipt.Append(npc.TableFilter, npc.MainChain,
			"-d", "224.0.0.0/4", "-j", "ACCEPT"); err != nil {
			return err
		}
	}

	// If the destination address is not any of the local pods, let it through
	if err := ipt.Append(npc.TableFilter, npc.MainChain,
		"-m", "physdev", "--physdev-is-bridged", "--physdev-out="+bridgePortName, "-j", "ACCEPT"); err != nil {
		return err
	}

	if err := ipt.Append(npc.TableFilter, npc.MainChain,
		"-m", "state", "--state", "NEW", "-j", string(npc.DefaultChain)); err != nil {
		return err
	}

	if err := ipt.Append(npc.TableFilter, npc.MainChain,
		"-m", "state", "--state", "NEW", "-j", string(npc.IngressChain)); err != nil {
		return err
	}

	if err := ipt.Append(npc.TableFilter, npc.EgressMarkChain,
		"-j", "MARK", "--set-xmark", npc.EgressMark); err != nil {
		return err
	}

	// Egress rules:
	//
	// -A WEAVE-NPC-EGRESS -m state --state RELATED,ESTABLISHED -j ACCEPT
	// -A WEAVE-NPC-EGRESS -m physdev --physdev-in vethwe-bridge --physdev-is-bridged -j RETURN
	// -A WEAVE-NPC-EGRESS -m addrtype --dst-type LOCAL -j RETURN
	// -A WEAVE-NPC-EGRESS -m state --state NEW -j WEAVE-NPC-EGRESS-DEFAULT
	// -A WEAVE-NPC-EGRESS -m state --state NEW -m mark ! --mark 0x40000/0x40000 -j WEAVE-NPC-EGRESS-CUSTOM
	// -A WEAVE-NPC-EGRESS -m state --state NEW -m mark ! --mark 0x40000/0x40000 -j NFLOG --nflog-group 86
	// -A WEAVE-NPC-EGRESS -m mark ! --mark 0x40000/0x40000 -j DROP
	//
	// -A WEAVE-NPC-EGRESS-CUSTOM <rulespec> -j MARK --set-xmark 0x40000/0x40000
	// -A WEAVE-NPC-EGRESS-CUSTOM <rulespec> -j RETURN
	//
	// -A WEAVE-NPC-EGRESS-DEFAULT <rulespec> -j MARK --set-xmark 0x40000/0x40000
	// -A WEAVE-NPC-EGRESS-DEFAULT <rulespec> -j RETURN
	//
	// For each rule we create two (mark and return). We cannot just accept
	// a packet if it matches any rule, as a packet might need to traverse
	// the ingress npc as well which happens later in the chain (in some cases
	// we cannot detect whether packet is ingress or egress, so we need to
	// check both chains).

	ruleSpecs := [][]string{
		{"-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		// skip running through egress network policies for the traffic not coming from local pods
		{"-m", "physdev", "--physdev-is-bridged", "--physdev-in=" + bridgePortName, "-j", "RETURN"},
		// skip running through egress network policies for the traffic bound for IP address assigned for the bridge
		{"-m", "addrtype", "--dst-type", "LOCAL", "-j", "RETURN"},
	}
	if allowMcast {
		ruleSpecs = append(ruleSpecs, []string{"-d", "224.0.0.0/4", "-j", "RETURN"})
	}
	ruleSpecs = append(ruleSpecs, [][]string{
		{"-m", "state", "--state", "NEW", "-j", string(npc.EgressDefaultChain)},
		{"-m", "state", "--state", "NEW", "-m", "mark", "!", "--mark", npc.EgressMark, "-j", string(npc.EgressCustomChain)},
		{"-m", "state", "--state", "NEW", "-m", "mark", "!", "--mark", npc.EgressMark, "-j", "NFLOG", "--nflog-group", "86"},
	}...)
	if err := net.AddChainWithRules(ipt, npc.TableFilter, npc.EgressChain, ruleSpecs); err != nil {
		return err
	}

	// delete `weave-local-pods` ipset which is no longer used by weave-npc
	weaveLocalPodExist, err := ipsetExist(ips, npc.LocalIpset)
	if err != nil {
		common.Log.Errorf("Failed to look if ipset '%s' exists", npc.LocalIpset)
	} else if weaveLocalPodExist {
		common.Log.Debugf("Destroying ipset '%s'", npc.LocalIpset)
		if err := ips.Destroy(npc.LocalIpset); err != nil {
			common.Log.Errorf("Failed to destroy ipset '%s'", npc.LocalIpset)
		}
	}

	return nil
}

// Dummy way to check whether a given ipset exists.
// TODO(brb) Use "ipset -exist create <..>" for our purpose instead (for some reasons
// creating an ipset with -exist fails).
func ipsetExist(ips ipset.Interface, name ipset.Name) (bool, error) {
	sets, err := ips.List(string(name))
	if err != nil {
		return false, err
	}
	for _, s := range sets {
		if s == name {
			return true, nil
		}
	}
	return false, nil
}

func root(cmd *cobra.Command, args []string) {
	var npController cache.Controller

	common.SetLogLevel(logLevel)
	if nodeName == "" {
		// HOSTNAME is set by Kubernetes for pods in the host network namespace
		nodeName = os.Getenv("HOSTNAME")
	}
	if nodeName == "" {
		common.Log.Fatalf("Must set node name via --node-name or $HOSTNAME")
	}
	common.Log.Infof("Starting Weaveworks NPC %s; node name %q", version, nodeName)

	if err := metrics.Start(metricsAddr); err != nil {
		common.Log.Fatalf("Failed to start metrics: %v", err)
	}

	if err := ulogd.Start(); err != nil {
		common.Log.Fatalf("Failed to start ulogd: %v", err)
	}

	config, err := rest.InClusterConfig()
	handleError(err)

	client, err := kubernetes.NewForConfig(config)
	handleError(err)

	ipt, err := iptables.New()
	handleError(err)

	ips := ipset.New(common.LogLogger(), maxList)

	handleError(resetIPTables(ipt))
	handleError(resetIPSets(ips))
	handleError(createBaseRules(ipt, ips))

	npc := npc.New(nodeName, ipt, ips, client)

	nsController := makeController(client.Core().RESTClient(), "namespaces", &coreapi.Namespace{},
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				handleError(npc.AddNamespace(obj.(*coreapi.Namespace)))
			},
			DeleteFunc: func(obj interface{}) {
				switch obj := obj.(type) {
				case *coreapi.Namespace:
					handleError(npc.DeleteNamespace(obj))
				case cache.DeletedFinalStateUnknown:
					// We know this object has gone away, but its final state is no longer
					// available from the API server. Instead we use the last copy of it
					// that we have, which is good enough for our cleanup.
					handleError(npc.DeleteNamespace(obj.Obj.(*coreapi.Namespace)))
				}
			},
			UpdateFunc: func(old, new interface{}) {
				handleError(npc.UpdateNamespace(old.(*coreapi.Namespace), new.(*coreapi.Namespace)))
			}})

	podController := makeController(client.Core().RESTClient(), "pods", &coreapi.Pod{},
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				handleError(npc.AddPod(obj.(*coreapi.Pod)))
			},
			DeleteFunc: func(obj interface{}) {
				switch obj := obj.(type) {
				case *coreapi.Pod:
					handleError(npc.DeletePod(obj))
				case cache.DeletedFinalStateUnknown:
					// We know this object has gone away, but its final state is no longer
					// available from the API server. Instead we use the last copy of it
					// that we have, which is good enough for our cleanup.
					handleError(npc.DeletePod(obj.Obj.(*coreapi.Pod)))
				}
			},
			UpdateFunc: func(old, new interface{}) {
				handleError(npc.UpdatePod(old.(*coreapi.Pod), new.(*coreapi.Pod)))
			}})

	npHandlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			handleError(npc.AddNetworkPolicy(obj))
		},
		DeleteFunc: func(obj interface{}) {
			switch obj := obj.(type) {
			case cache.DeletedFinalStateUnknown:
				// We know this object has gone away, but its final state is no longer
				// available from the API server. Instead we use the last copy of it
				// that we have, which is good enough for our cleanup.
				handleError(npc.DeleteNetworkPolicy(obj.Obj))
			default:
				handleError(npc.DeleteNetworkPolicy(obj))
			}
		},
		UpdateFunc: func(old, new interface{}) {
			handleError(npc.UpdateNetworkPolicy(old, new))
		},
	}
	npController = makeController(client.NetworkingV1().RESTClient(), "networkpolicies", &networkingv1.NetworkPolicy{}, npHandlers)

	go nsController.Run(wait.NeverStop)
	go podController.Run(wait.NeverStop)
	go npController.Run(wait.NeverStop)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	common.Log.Fatalf("Exiting: %v", <-signals)
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "weave-npc",
		Short: "Weaveworks Kubernetes Network Policy Controller",
		Run:   root}

	rootCmd.PersistentFlags().StringVar(&metricsAddr, "metrics-addr", ":6781", "metrics server bind address")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "debug", "logging level (debug, info, warning, error)")
	rootCmd.PersistentFlags().BoolVar(&allowMcast, "allow-mcast", true, "allow all multicast traffic")
	rootCmd.PersistentFlags().StringVar(&nodeName, "node-name", "", "only generate rules that apply to this node")
	rootCmd.PersistentFlags().IntVar(&maxList, "max-list-size", 1024, "maximum size of ipset list (for namespaces)")
	rootCmd.PersistentFlags().StringVar(&bridgePortName, "bridge-port-name", "vethwe-bridge", "name of the brige port on which packets are received and sent")

	handleError(rootCmd.Execute())
}
