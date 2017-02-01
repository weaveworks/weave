package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	coreapi "k8s.io/client-go/pkg/api/v1"
	extnapi "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/fields"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc"
	"github.com/weaveworks/weave/npc/ipset"
	"github.com/weaveworks/weave/npc/metrics"
	"github.com/weaveworks/weave/npc/ulogd"
)

var (
	version     = "unreleased"
	metricsAddr string
	logLevel    string
	allowMcast  bool
)

func handleError(err error) { common.CheckFatal(err) }

func makeController(getter cache.Getter, resource string,
	objType runtime.Object, handlers cache.ResourceEventHandlerFuncs) *cache.Controller {
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

	if err := ipt.Append(npc.TableFilter, npc.MainChain,
		"-m", "state", "--state", "NEW", "-j", string(npc.DefaultChain)); err != nil {
		return err
	}

	if err := ipt.Append(npc.TableFilter, npc.MainChain,
		"-m", "state", "--state", "NEW", "-j", string(npc.IngressChain)); err != nil {
		return err
	}

	return nil
}

func resetIPSets(ips ipset.Interface) error {
	// TODO should restrict ipset operations to the `weave-` prefix:

	if err := ips.FlushAll(); err != nil {
		return err
	}

	if err := ips.DestroyAll(); err != nil {
		return err
	}

	return nil
}

func root(cmd *cobra.Command, args []string) {
	common.SetLogLevel(logLevel)
	common.Log.Infof("Starting Weaveworks NPC %s", version)

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

	ips := ipset.New()

	handleError(resetIPTables(ipt))
	handleError(resetIPSets(ips))

	npc := npc.New(ipt, ips)

	nsController := makeController(client.Core().RESTClient(), "namespaces", &coreapi.Namespace{},
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				handleError(npc.AddNamespace(obj.(*coreapi.Namespace)))
			},
			DeleteFunc: func(obj interface{}) {
				handleError(npc.DeleteNamespace(obj.(*coreapi.Namespace)))
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
				handleError(npc.DeletePod(obj.(*coreapi.Pod)))
			},
			UpdateFunc: func(old, new interface{}) {
				handleError(npc.UpdatePod(old.(*coreapi.Pod), new.(*coreapi.Pod)))
			}})

	npController := makeController(client.Extensions().RESTClient(), "networkpolicies", &extnapi.NetworkPolicy{},
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				handleError(npc.AddNetworkPolicy(obj.(*extnapi.NetworkPolicy)))
			},
			DeleteFunc: func(obj interface{}) {
				handleError(npc.DeleteNetworkPolicy(obj.(*extnapi.NetworkPolicy)))
			},
			UpdateFunc: func(old, new interface{}) {
				handleError(npc.UpdateNetworkPolicy(old.(*extnapi.NetworkPolicy), new.(*extnapi.NetworkPolicy)))
			}})

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

	handleError(rootCmd.Execute())
}
