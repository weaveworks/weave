package nameserver

import (
	"github.com/fsouza/go-dockerclient"
)

func checkError(err error, apiPath string) {
	if err != nil {
		Error.Fatalf("Unable to connect to Docker API on %s: %s", apiPath, err)
	}
}

func StartUpdater(apiPath string, zone Zone) error {
	client, err := docker.NewClient(apiPath)
	checkError(err, apiPath)

	env, err := client.Version()
	checkError(err, apiPath)

	events := make(chan *docker.APIEvents)
	err = client.AddEventListener(events)
	checkError(err, apiPath)

	Info.Printf("Using Docker API on %s: %v", apiPath, env)

	go func() {
		for event := range events {
			handleEvent(zone, event, client)
		}
	}()
	return nil
}

func handleEvent(zone Zone, event *docker.APIEvents, client *docker.Client) error {
	switch event.Status {
	case "die":
		id := event.ID
		Info.Printf("Container %s down. Removing records", id)
		zone.DeleteRecordsFor(id)
	}
	return nil
}
