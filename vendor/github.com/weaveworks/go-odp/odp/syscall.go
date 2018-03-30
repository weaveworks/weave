package odp

import "syscall"

// from linux/include/linux/socket.h
const SOL_NETLINK = 270

type GenlMsghdr struct {
	Cmd      uint8
	Version  uint8
	Reserved uint16
}

const SizeofGenlMsghdr = 4

// reserved static generic netlink identifiers:
const (
	GENL_ID_GENERATE  = 0
	GENL_ID_CTRL      = syscall.NLMSG_MIN_TYPE
	GENL_ID_VFS_DQUOT = syscall.NLMSG_MIN_TYPE + 1
	GENL_ID_PMCRAID   = syscall.NLMSG_MIN_TYPE + 2
)

const (
	CTRL_CMD_UNSPEC       = 0
	CTRL_CMD_NEWFAMILY    = 1
	CTRL_CMD_DELFAMILY    = 2
	CTRL_CMD_GETFAMILY    = 3
	CTRL_CMD_NEWOPS       = 4
	CTRL_CMD_DELOPS       = 5
	CTRL_CMD_GETOPS       = 6
	CTRL_CMD_NEWMCAST_GRP = 7
	CTRL_CMD_DELMCAST_GRP = 8
)

const (
	CTRL_ATTR_UNSPEC       = 0
	CTRL_ATTR_FAMILY_ID    = 1
	CTRL_ATTR_FAMILY_NAME  = 2
	CTRL_ATTR_VERSION      = 3
	CTRL_ATTR_HDRSIZE      = 4
	CTRL_ATTR_MAXATTR      = 5
	CTRL_ATTR_OPS          = 6
	CTRL_ATTR_MCAST_GROUPS = 7
)

const (
	CTRL_ATTR_MCAST_GRP_UNSPEC = 0
	CTRL_ATTR_MCAST_GRP_NAME   = 1
	CTRL_ATTR_MCAST_GRP_ID     = 2
)

type OvsHeader struct {
	DpIfIndex int32
}

const SizeofOvsHeader = 4

const (
	OVS_DATAPATH_VERSION = 2
	OVS_VPORT_VERSION    = 1
	OVS_FLOW_VERSION     = 1
	OVS_PACKET_VERSION   = 1
)

const ( // ovs_datapath_cmd
	OVS_DP_CMD_UNSPEC = 0
	OVS_DP_CMD_NEW    = 1
	OVS_DP_CMD_DEL    = 2
	OVS_DP_CMD_GET    = 3
	OVS_DP_CMD_SET    = 4
)

const ( // ovs_datapath_attr
	OVS_DP_ATTR_UNSPEC         = 0
	OVS_DP_ATTR_NAME           = 1
	OVS_DP_ATTR_UPCALL_PID     = 2
	OVS_DP_ATTR_STATS          = 3
	OVS_DP_ATTR_MEGAFLOW_STATS = 4
	OVS_DP_ATTR_USER_FEATURES  = 5
)

const (
	OVS_DP_F_UNALIGNED  = 1
	OVS_DP_F_VPORT_PIDS = 2
)

const ( // ovs_vport_cmd
	OVS_VPORT_CMD_UNSPEC = 0
	OVS_VPORT_CMD_NEW    = 1
	OVS_VPORT_CMD_DEL    = 2
	OVS_VPORT_CMD_GET    = 3
	OVS_VPORT_CMD_SET    = 4
)

const ( // ovs_vport_attr
	OVS_VPORT_ATTR_UNSPEC     = 0
	OVS_VPORT_ATTR_PORT_NO    = 1
	OVS_VPORT_ATTR_TYPE       = 2
	OVS_VPORT_ATTR_NAME       = 3
	OVS_VPORT_ATTR_OPTIONS    = 4
	OVS_VPORT_ATTR_UPCALL_PID = 5
	OVS_VPORT_ATTR_STATS      = 6
)

const ( // ovs_vport_type
	OVS_VPORT_TYPE_UNSPEC   = 0
	OVS_VPORT_TYPE_NETDEV   = 1
	OVS_VPORT_TYPE_INTERNAL = 2
	OVS_VPORT_TYPE_GRE      = 3
	OVS_VPORT_TYPE_VXLAN    = 4
	OVS_VPORT_TYPE_GENEVE   = 5
)

const ( // OVS_VPORT_ATTR_OPTIONS attributes for tunnels
	OVS_TUNNEL_ATTR_UNSPEC   = 0
	OVS_TUNNEL_ATTR_DST_PORT = 1
)

const ( // ovs_flow_cmd
	OVS_FLOW_CMD_UNSPEC = 0
	OVS_FLOW_CMD_NEW    = 1
	OVS_FLOW_CMD_DEL    = 2
	OVS_FLOW_CMD_GET    = 3
	OVS_FLOW_CMD_SET    = 4
)

const ( // ovs_flow_attr
	OVS_FLOW_ATTR_UNSPEC    = 0
	OVS_FLOW_ATTR_KEY       = 1
	OVS_FLOW_ATTR_ACTIONS   = 2
	OVS_FLOW_ATTR_STATS     = 3
	OVS_FLOW_ATTR_TCP_FLAGS = 4
	OVS_FLOW_ATTR_USED      = 5
	OVS_FLOW_ATTR_CLEAR     = 6
	OVS_FLOW_ATTR_MASK      = 7
)

type OvsFlowStats struct {
	NPackets uint64
	NBytes   uint64
}

const SizeofOvsFlowStats = 16

const ( // ovs_key_attr
	OVS_KEY_ATTR_UNSPEC    = 0
	OVS_KEY_ATTR_ENCAP     = 1
	OVS_KEY_ATTR_PRIORITY  = 2
	OVS_KEY_ATTR_IN_PORT   = 3
	OVS_KEY_ATTR_ETHERNET  = 4
	OVS_KEY_ATTR_VLAN      = 5
	OVS_KEY_ATTR_ETHERTYPE = 6
	OVS_KEY_ATTR_IPV4      = 7
	OVS_KEY_ATTR_IPV6      = 8
	OVS_KEY_ATTR_TCP       = 9
	OVS_KEY_ATTR_UDP       = 10
	OVS_KEY_ATTR_ICMP      = 11
	OVS_KEY_ATTR_ICMPV6    = 12
	OVS_KEY_ATTR_ARP       = 13
	OVS_KEY_ATTR_ND        = 14
	OVS_KEY_ATTR_SKB_MARK  = 15
	OVS_KEY_ATTR_TUNNEL    = 16
	OVS_KEY_ATTR_SCTP      = 17
	OVS_KEY_ATTR_TCP_FLAGS = 18
	OVS_KEY_ATTR_DP_HASH   = 19
	OVS_KEY_ATTR_RECIRC_ID = 20
)

const ( // ovs_tunnel_key_attr
	OVS_TUNNEL_KEY_ATTR_ID            = 0
	OVS_TUNNEL_KEY_ATTR_IPV4_SRC      = 1
	OVS_TUNNEL_KEY_ATTR_IPV4_DST      = 2
	OVS_TUNNEL_KEY_ATTR_TOS           = 3
	OVS_TUNNEL_KEY_ATTR_TTL           = 4
	OVS_TUNNEL_KEY_ATTR_DONT_FRAGMENT = 5
	OVS_TUNNEL_KEY_ATTR_CSUM          = 6
	OVS_TUNNEL_KEY_ATTR_OAM           = 7
	OVS_TUNNEL_KEY_ATTR_GENEVE_OPTS   = 8
	OVS_TUNNEL_KEY_ATTR_TP_SRC        = 9
	OVS_TUNNEL_KEY_ATTR_TP_DST        = 10
	OVS_TUNNEL_KEY_ATTR_VXLAN_OPTS    = 11
	OVS_TUNNEL_KEY_ATTR_IPV6_SRC      = 12
	OVS_TUNNEL_KEY_ATTR_IPV6_DST      = 13
)

const ETH_ALEN = 6

type OvsKeyEthernet struct {
	EthSrc [ETH_ALEN]byte
	EthDst [ETH_ALEN]byte
}

const SizeofOvsKeyEthernet = 12

const ( // ovs_action_attr
	OVS_ACTION_ATTR_UNSPEC    = 0
	OVS_ACTION_ATTR_OUTPUT    = 1
	OVS_ACTION_ATTR_USERSPACE = 2
	OVS_ACTION_ATTR_SET       = 3
	OVS_ACTION_ATTR_PUSH_VLAN = 4
	OVS_ACTION_ATTR_POP_VLAN  = 5
	OVS_ACTION_ATTR_SAMPLE    = 6
)

const ( // ovs_packet_cmd
	OVS_PACKET_CMD_UNSPEC  = 0
	OVS_PACKET_CMD_MISS    = 1
	OVS_PACKET_CMD_ACTION  = 2
	OVS_PACKET_CMD_EXECUTE = 3
)

const ( // ovs_packet_attr
	OVS_PACKET_ATTR_UNSPEC   = 0
	OVS_PACKET_ATTR_PACKET   = 1
	OVS_PACKET_ATTR_KEY      = 2
	OVS_PACKET_ATTR_ACTIONS  = 3
	OVS_PACKET_ATTR_USERDATA = 4
)

type ifreqIfindex struct {
	name    [syscall.IFNAMSIZ]byte
	ifindex int32
}
