package proxy

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

const MaxDockerHostname = 64

var (
	ErrNoCommandSpecified = errors.New("No command specified")
)

type createContainerInterceptor struct{ proxy *Proxy }

type createContainerRequestBody struct {
	*docker.Config
	HostConfig *docker.HostConfig `json:"HostConfig,omitempty" yaml:"HostConfig,omitempty"`
	MacAddress string             `json:"MacAddress,omitempty" yaml:"MacAddress,omitempty"`
}

// ErrNoSuchImage replaces docker.NoSuchImage, which does not contain the image
// name, which in turn breaks docker clients post 1.7.0 since they expect the
// image name to be present in errors.
type ErrNoSuchImage struct {
	Name string
}

func (err *ErrNoSuchImage) Error() string {
	return "No such image: " + err.Name
}

func (i *createContainerInterceptor) InterceptRequest(r *http.Request) error {
	container := createContainerRequestBody{}
	if err := unmarshalRequestBody(r, &container); err != nil {
		return err
	}

	if cidrs, err := i.proxy.weaveCIDRsFromConfig(container.Config, container.HostConfig); err != nil {
		Log.Infof("Leaving container alone because %s", err)
	} else {
		Log.Infof("Creating container with WEAVE_CIDR \"%s\"", strings.Join(cidrs, " "))
		if container.HostConfig == nil {
			container.HostConfig = &docker.HostConfig{}
		}
		if container.Config == nil {
			container.Config = &docker.Config{}
		}
		i.proxy.addWeaveWaitVolume(container.HostConfig)
		if err := i.setWeaveWaitEntrypoint(container.Config); err != nil {
			return err
		}
		hostname := r.URL.Query().Get("name")
		if i.proxy.Config.HostnameFromLabel != "" {
			if labelValue, ok := container.Labels[i.proxy.Config.HostnameFromLabel]; ok {
				hostname = labelValue
			}
		}
		hostname = i.proxy.hostnameMatchRegexp.ReplaceAllString(hostname, i.proxy.HostnameReplacement)
		if dnsDomain, withDNS := i.proxy.getDNSDomain(); withDNS {
			i.setHostname(&container, hostname, dnsDomain)
			if err := i.proxy.setWeaveDNS(container.HostConfig, container.Hostname, dnsDomain); err != nil {
				return err
			}
		}

		return marshalRequestBody(r, container)
	}

	return nil
}

func (i *createContainerInterceptor) setWeaveWaitEntrypoint(container *docker.Config) error {
	if len(container.Entrypoint) == 0 {
		image, err := i.proxy.client.InspectImage(container.Image)
		if err == docker.ErrNoSuchImage {
			return &ErrNoSuchImage{container.Image}
		} else if err != nil {
			return err
		}

		if len(container.Cmd) == 0 {
			container.Cmd = image.Config.Cmd
		}

		if container.Entrypoint == nil {
			container.Entrypoint = image.Config.Entrypoint
		}
	}

	if len(container.Entrypoint) == 0 && len(container.Cmd) == 0 {
		return ErrNoCommandSpecified
	}

	if len(container.Entrypoint) == 0 || container.Entrypoint[0] != weaveWaitEntrypoint[0] {
		container.Entrypoint = append(weaveWaitEntrypoint, container.Entrypoint...)
	}

	return nil
}

func (i *createContainerInterceptor) setHostname(container *createContainerRequestBody, name, dnsDomain string) {
	if container.Hostname == "" && name != "" {
		// Strip trailing period because it's unusual to see it used on the end of a host name
		trimmedDNSDomain := strings.TrimSuffix(dnsDomain, ".")
		if len(name)+1+len(trimmedDNSDomain) > MaxDockerHostname {
			Log.Warningf("Container name [%s] too long to be used as hostname", name)
		} else {
			container.Hostname = name
			container.Domainname = trimmedDNSDomain
		}
	}

	return
}

func (i *createContainerInterceptor) InterceptResponse(r *http.Response) error {
	return nil
}
