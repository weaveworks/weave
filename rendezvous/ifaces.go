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
	"^weave.*",
	"^dummy.*",
	"^ethwe.*",
	"^veth.*",
	"^vnet.*",
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

// An interfaces names set
type IfaceNamesList map[string]bool

func NewIfaceNamesList() IfaceNamesList {
	return make(map[string]bool)
}

func (il *IfaceNamesList) String() string {
	return fmt.Sprint(*il)
}

func (il *IfaceNamesList) ToInterfaces() ([]*net.Interface, error) {
	parsedIfaces := make([]*net.Interface, 0)
	for iface := range *il {
		parsedIface, err := net.InterfaceByName(iface)
		if err != nil {
			return nil, fmt.Errorf("Could not find interface %s", iface)
		}
		parsedIfaces = append(parsedIfaces, parsedIface)
	}
	return parsedIfaces, nil
}

// Utility method for setting the interfaces from a list of comma-separated interfaces
// Note that we do not remove previously set values.
func (il IfaceNamesList) Set(value string) error {
	for _, ifacestr := range strings.Split(value, ",") {
		il[ifacestr] = true
	}
	return nil
}

// Guess the external interfaces, ignoring some devices by default, and some others if
// they are in `extraIgnored`
func GuessExternalInterfaces(extraIgnored IfaceNamesList) ([]*net.Interface, error) {
	parsedIfaces := make([]*net.Interface, 0)

	Debug.Printf("Guessing external interfaces...")
	guessedIfaces, err := net.Interfaces()
	if err != nil {
		return nil, errIfaceListError
	}

	isExtraIgnored := func(i *net.Interface) bool {
		_, inExtra := extraIgnored[i.Name]
		return inExtra
	}

	for idx, _ := range guessedIfaces {
		guessedIface := &guessedIfaces[idx]
		if !isIgnoredIface(guessedIface) && !isExtraIgnored(guessedIface) {
			Debug.Printf("... %s seems a valid interface", guessedIface.Name)
			parsedIfaces = append(parsedIfaces, guessedIface)
		}
	}

	return parsedIfaces, nil
}

// A pair of interface and corresponding IP address
type externalIface struct {
	iface *net.Interface
	ip    net.IP
}

func (re *externalIface) String() string {
	return fmt.Sprintf("%s@%s", re.ip.String(), re.iface.Name)
}

// For each interface, create an endpoint: a pair of interface and external IP address
func ExternalsFromIfaces(ifaces []*net.Interface) ([]externalIface, error) {
	eps := make([]externalIface, 0)
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				Error.Printf("Could not parse interface address %s", addr)
				continue
			}
			if ip.IsGlobalUnicast() {
				eps = append(eps, externalIface{iface, ip})
			}
		}
	}

	return eps, nil
}
