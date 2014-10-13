package main

import (
	"flag"
	"github.com/fsouza/go-dockerclient"
	"log"
)

func main() {
	var (
		serverAddr string
		serverPort int
		apiPath    string
	)
	flag.StringVar(&serverAddr, "host", "", "address or hostname of server to update")
	flag.IntVar(&serverPort, "port", 6785, "port on server to send updates to")
	flag.StringVar(&apiPath, "api", "unix:///var/run/docker.sock",
		"Path to Docker API socket")
	flag.Parse()

	client, err := docker.NewClient(apiPath)
	if err != nil {
		log.Fatalf("Unable to connect to Docker API on %s: %s", apiPath, err)
	}

	events := make(chan *docker.APIEvents)
	done := make(chan bool)
	client.AddEventListener(events)

	log.Printf("Using Docker API on %s", apiPath)
	log.Printf("Posting updates to server %s:%d", serverAddr, serverPort)

	go func() {
		for event := range events {
			log.Printf("Event %s", *event)
		}
		done <- true
	}()
	<-done
}
