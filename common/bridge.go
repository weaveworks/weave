package common

import "fmt"
import "github.com/vishvananda/netlink"
import "github.com/weaveworks/weave/common/odp"

type BridgeType int

const (
	None BridgeType = iota
	Bridge
	Fastdp
	BridgedFastdp
	Inconsistent
)

type BridgeConfig struct {
	DockerBridgeName string
	WeaveBridgeName  string
	DatapathName     string
	NoFastdp         bool
	NoBridgedFastdp  bool
	MTU              int
}

type bridgeContext struct {
	WeaveBridge netlink.Link
	Datapath    netlink.Link
}

func CreateBridge(config *BridgeConfig) (BridgeType, error) {
	context = &bridgeContext{}

	bridgeType := detectBridgeType(config, context)

	if bridgeType == None {
		bridgeType = Bridge
		if !config.NoFastdp {
			bridgeType = BridgedFastdp
			if !config.NoBridgedFastdp {
				bridgeType = Fastdp
				config.DatapathName = config.WeaveBridgeName
			}
			odpSupported, err := odp.CreateDatapath(config.DatapathName)
			if err != nil {
				return None, err
			}
			if !odpSupported {
				bridgeType = Bridge
			}
		}

		var err error
		switch bridgeType {
		case Bridge:
			err = initBridge(config, context)
		case Fastdp:
			err = initFastdp(config, context)
		case BridgedFastdp:
			err = initBridgedFastdp(config, context)
		default:
			err = fmt.Errorf("Cannot initialise bridge type %v", bridgeType)
		}
		if err != nil {
			return None, err
		}

		configureIPTables(config, context)
	}

	if bridgeType == Bridge {
		if err := EthtoolTXOff(config.WeaveBridgeName); err != nil {
			return bridgeType, err
		}
	}

	if err := linkSetUpByName(config.WeaveBridgeName); err != nil {
		return bridgeType, err
	}

	if err := ConfigureARPCache(config.WeaveBridgeName); err != nil {
		return bridgeType, err
	}

	return bridgeType, nil
}

func detectBridgeType(config *BridgeConfig, context *bridgeContext) BridgeType {
	bridge, _ := netlink.LinkByName(config.WeaveBridgeName)
	datapath, _ := netlink.LinkByName(config.DatapathName)

	context.bridge = bridge
	context.datapath = datapath

	switch {
	case bridge == nil && datapath == nil:
		return None
	case isBridge(bridge) && datapath == nil:
		return Bridge
	case isDatapath(bridge) && datapath == nil:
		return Fastdp
	case isDatapath(datapath) && isBridge(bridge):
		return BridgedFastdp
	default:
		return Inconsistent
	}
}

func isBridge(link netlink.Link) bool {
	_, isBridge := link.(*netlink.Bridge)
	return isBridge
}

func isDatapath(link netlink.Link) bool {
	switch link.(type) {
	case *netlink.GenericLink:
		return link.Type() == "openvswitch"
	case *netlink.Device:
		return true
	default:
		return false
	}
}

func initBridge(config BridgeConfig) error {
	mac, err := PersistentMAC()
	if err != nil {
		mac, err = RandomMAC()
		if err != nil {
			return err
		}
	}

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = config.WeaveBridgeName
	linkAttrs.HardwareAddr = mac
	linkAttrs.MTU = config.MTU // TODO this probably doesn't work - see weave script
	netlink.LinkAdd(&netlink.Bridge{linkAttrs})

	return nil
}

func initFastdp(config BridgeConfig) error {
	datapath, err := netlink.LinkByName(config.DatapathName)
	if err != nil {
		return err
	}
	return netlink.LinkSetMTU(datapath, config.MTU)
}

func initBridgedFastdp(config BridgeConfig) error {
	if err := initFastdp(config); err != nil {
		return err
	}
	if err := initBridge(config); err != nil {
		return err
	}

	link := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: "vethwe-bridge",
			MTU:  config.MTU},
		PeerName: "vethwe-datapath",
	}

	if err := netlink.LinkAdd(link); err != nil {
		return err
	}

	bridge, err := netlink.LinkByName(config.WeaveBridgeName)
	if err != nil {
		return err
	}

	if err := netlink.LinkSetMasterByIndex(link, bridge.Attrs().Index); err != nil {
		return err
	}

	if err := odp.AddDatapathInterface(config.DatapathName, "vethwe-datapath"); err != nil {
		return err
	}

	if err := linkSetUpByName(config.DatapathName); err != nil {
		return err
	}

	return nil
}

func configureIPTables(config BridgeConfig) error {
	return fmt.Errorf("Not implemented")
}

func linkSetUpByName(linkName string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}
	return netlink.LinkSetUp(link)
}
