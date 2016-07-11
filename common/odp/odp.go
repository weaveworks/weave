package odp

import (
	"fmt"
	"net"
	"syscall"

	"github.com/weaveworks/go-odp/odp"
)

// ODP admin functionality

func CreateDatapath(dpname string) (supported bool, err error) {
	dpif, err := odp.NewDpif()
	if err != nil {
		if odp.IsKernelLacksODPError(err) {
			return false, nil
		}
		return true, err
	}
	defer dpif.Close()

	dp, err := dpif.CreateDatapath(dpname)
	if err != nil && !odp.IsDatapathNameAlreadyExistsError(err) {
		return true, err
	}

	vpid, _, err := CreateDummyVxlanVport(dpname, true)
	if nlerr, ok := err.(odp.NetlinkError); ok {
		if syscall.Errno(nlerr) == syscall.EAFNOSUPPORT {
			dp.Delete()
			return false, fmt.Errorf("kernel does not have Open vSwitch VXLAN support")
		}
	}
	if err == nil {
		dp.DeleteVport(vpid)
	}

	return true, nil
}

func DeleteDatapath(dpname string) error {
	dpif, err := odp.NewDpif()
	if err != nil {
		return err
	}
	defer dpif.Close()

	dp, err := dpif.LookupDatapath(dpname)
	if err != nil {
		if odp.IsNoSuchDatapathError(err) {
			return nil
		}
		return err
	}

	return dp.Delete()
}

func AddDatapathInterface(dpname string, ifname string) error {
	dpif, err := odp.NewDpif()
	if err != nil {
		return err
	}
	defer dpif.Close()

	dp, err := dpif.LookupDatapath(dpname)
	if err != nil {
		return err
	}

	_, err = dp.CreateVport(odp.NewNetdevVportSpec(ifname))
	return err
}

func CreateDummyVxlanVport(dpname string, keepUDPSocket bool) (odp.VportID, string, error) {
	dpif, err := odp.NewDpif()
	if err != nil {
		return 0, "", err
	}
	defer dpif.Close()

	dp, err := dpif.LookupDatapath(dpname)
	if err != nil {
		return 0, "", err
	}

	// A dummy way to get an ephemeral port for UDP
	udpconn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return 0, "", err
	}
	portno := uint16(udpconn.LocalAddr().(*net.UDPAddr).Port)
	if keepUDPSocket {
		// we leave the UDP socket open, so creating a vxlan vport on
		// the same port number should fail.  But that's fine: It's
		// still sufficient to probe for support.
		defer udpconn.Close()
	} else {
		udpconn.Close()
	}

	vpname := fmt.Sprintf("vxlan-%d", portno)
	vpid, err := dp.CreateVport(odp.NewVxlanVportSpec(vpname, portno))

	return vpid, vpname, err
}

func DeleteVport(dpname string, vpid odp.VportID) error {
	dpif, err := odp.NewDpif()
	if err != nil {
		return err
	}
	defer dpif.Close()

	dp, err := dpif.LookupDatapath(dpname)
	if err != nil {
		return err
	}

	return dp.DeleteVport(vpid)
}
