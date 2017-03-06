package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// TODO: move these definitions somewhere more shareable
type PeerUpdateRequest struct {
	Name      string   `json:"peername"`
	Nickname  string   `json:"nickname"`  // optional
	Addresses []string `json:"addresses"` // can be empty
}

type PeerUpdateResponse struct {
	Addresses []string `json:"addresses"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func do(verb string, discoveryEndpoint, token string, request interface{}, response interface{}) error {
	body := new(bytes.Buffer)
	err := json.NewEncoder(body).Encode(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(verb, discoveryEndpoint+"/peer", body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Scope-Probe token=%s", token))
	req.Header.Set("X-Weave-Net-Version", version)
	req.Header.Set("Content-Type", "application/json")
	Log.Printf("Calling peer discovery %s with %s", req.URL, request)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		rbody, _ := ioutil.ReadAll(resp.Body)
		return errors.New(resp.Status + ": " + string(rbody))
	}
	if response == nil {
		return nil
	}
	err = json.NewDecoder(resp.Body).Decode(response)
	Log.Printf("peer discovery result: (%s) %v", err, response)
	return err
}

func peerDiscoveryUpdate(discoveryEndpoint, token, peername, nickname string, addresses []string) ([]string, error) {
	request := PeerUpdateRequest{
		Name:      peername,
		Nickname:  nickname,
		Addresses: addresses,
	}
	var updateResponse PeerUpdateResponse
	err := do("POST", discoveryEndpoint, token, request, &updateResponse)
	return updateResponse.Addresses, err
}
