package updater

import (
	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type ContainerObserver interface {
	ContainerDied(ident string) error
}

func checkError(err error, apiPath string) {
	if err != nil {
		Error.Fatalf("[updater] Unable to connect to Docker API on %s: %s",
			apiPath, err)
	}
}

func Start(apiPath string, ob ContainerObserver) error {
	client, err := docker.NewClient(apiPath)
	checkError(err, apiPath)

	env, err := client.Version()
	checkError(err, apiPath)

	events := make(chan *docker.APIEvents)
	err = client.AddEventListener(events)
	checkError(err, apiPath)

	Info.Printf("[updater] Using Docker API on %s: %v", apiPath, env)

	go func() {
		for event := range events {
			handleEvent(ob, event, client)
		}
	}()
	return nil
}

func handleEvent(ob ContainerObserver, event *docker.APIEvents, client *docker.Client) error {
	switch event.Status {
	case "die":
		id := event.ID
		ob.ContainerDied(id)
	}
	return nil
}
