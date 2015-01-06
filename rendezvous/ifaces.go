package rendezvous

import (
	"errors"
	"fmt"
	. "github.com/zettio/weave/common"
	"net"
	"regexp"
	"strings"
)

// Error when gathering interfaces list
var errIfaceListError = errors.New("Error when gathering interfaces list")

// (regular expressions of) interfaces we ignore when calculating the external IPs
var ignoredIfaces = []string{
	"^ethwe.*",
	"^bridge.*",
	"^docker.*",
	"^tun.*",
	"^tap.*",
	"^virbr.*",
	"^lo.*",
}

// check if a interface must be ignored
func isIgnoredIface(i *net.Interface) bool {
	iface := i.Name
	for _, ignoredRegexp := range ignoredIfaces {
		// (we do not check for errors in the regexps!)
		if matched, _ := regexp.MatchString(ignoredRegexp, iface); matched {
			return true
		}
	}
	return false
}

// External interfaces
type IfaceNamesList map[string]bool

func NewIfaceNamesList() IfaceNamesList {
	return make(map[string]bool)
}
func (il *IfaceNamesList) String() string {
	return fmt.Sprint(*il)
}

// Utility method for setting the interfaces from a list of comma-separated interfaces
// Note that we do not remove previously set values.
func (il IfaceNamesList) Set(value string) error {
	for _, ifacestr := range strings.Split(value, ",") {
		il[ifacestr] = true
	}
	return nil
}

// A pair of interface and corresponding IP address
type RendezvousEndpoint struct {
	iface *net.Interface
	ip    net.IP
}

func (re *RendezvousEndpoint) String() string {
	return fmt.Sprintf("%s@%s", re.ip.String(), re.iface.Name)
}

// Convert the interfaces list provided in command line to a list of endpoints
func EndpointsListFromIfaceNamesList(ifaces IfaceNamesList) ([]RendezvousEndpoint, error) {
	parsedIfaces := make([]*net.Interface, 0)

	if len(ifaces) > 0 {
		for iface := range ifaces {
			parsedIface, err := net.InterfaceByName(iface)
			if err != nil {
				return nil, fmt.Errorf("Could not find interface %s", iface)
			}
			parsedIfaces = append(parsedIfaces, parsedIface)
		}
	} else {
		Debug.Printf("Guessing external devices list...")
		guessedIfaces, err := net.Interfaces()
		if err != nil {
			return nil, errIfaceListError
		}

		for idx, _ := range guessedIfaces {
			guessedIface := &guessedIfaces[idx]
			if !isIgnoredIface(guessedIface) {
				parsedIfaces = append(parsedIfaces, guessedIface)
			}
		}
	}

	// for each interface, create an entrypoint: a pair of interface and external IP address
	eps := make([]RendezvousEndpoint, 0)
	for _, iface := range parsedIfaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				Error.Printf("Could not parse device address %s", addr)
				continue
			}
			if ip.IsGlobalUnicast() {
				eps = append(eps, RendezvousEndpoint{
					iface: iface,
					ip:    ip,
				})
			}
		}
	}

	return eps, nil
}
