package main

import (
	"flag"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"log"
	"net/http"
)

func main() {
	var (
		updatePath string
		apiPath    string
	)
	flag.StringVar(&updatePath, "endpoint", "", "Endpoint to which to send updates")
	flag.StringVar(&apiPath, "api", "unix:///var/run/docker.sock", "Path to Docker API socket")
	flag.Parse()

	// TODO: check status on endpoint

	client, err := docker.NewClient(apiPath)
	if err != nil {
		log.Fatalf("Unable to connect to Docker API on %s: %s", apiPath, err)
	}

	events := make(chan *docker.APIEvents)
	done := make(chan bool)
	client.AddEventListener(events)

	log.Printf("Using Docker API on %s", apiPath)
	log.Printf("Posting updates to ", updatePath)

	go func() {
		for event := range events {
			handleEvent(event, client, updatePath)
		}
		done <- true
	}()
	<-done
}

func handleEvent(event *docker.APIEvents, client *docker.Client, endpoint string) error {
	switch event.Status {
	case "die":
		id := event.ID
		url := fmt.Sprintf("%s/name/%s", endpoint, id)
		client := &http.Client{}
		req, _ := http.NewRequest("DELETE", url, nil)
		client.Do(req)
	}
	return nil
}
