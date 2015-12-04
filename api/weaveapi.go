package api

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	. "github.com/weaveworks/weave/common"
)

const (
	WeaveHttpPort = 6784
)

type Client struct {
	baseUrl string
}

func (client *Client) httpVerb(verb string, url string, values url.Values) (string, error) {
	url = client.baseUrl + url
	Log.Debugf("weave %s to %s with %v", verb, url, values)
	var body io.Reader
	if values != nil {
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequest(verb, url, body)
	if err != nil {
		return "", err
	}
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if resp, err := http.DefaultClient.Do(req); err != nil {
		return "", err
	} else {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			return string(body), nil
		} else {
			return "", errors.New(resp.Status + ": " + string(body))
		}
	}
}

func NewClient(addr string) *Client {
	return &Client{baseUrl: fmt.Sprintf("http://%s:%d", addr, WeaveHttpPort)}
}

func (client *Client) Connect(remote string) error {
	_, err := client.httpVerb("POST", "/connect", url.Values{"peer": {remote}})
	return err
}
