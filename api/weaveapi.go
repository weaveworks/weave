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
	WeaveHTTPPort = 6784
)

type Client struct {
	baseURL string
	resolve func() (string, error)
}

func (client *Client) httpVerb(verb string, url string, values url.Values) (string, error) {
	baseURL := client.baseURL
	if client.resolve != nil {
		addr, err := client.resolve()
		if err != nil {
			return "", err
		}
		baseURL = fmt.Sprintf("http://%s:%d", addr, WeaveHTTPPort)
	}
	url = baseURL + url
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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rbody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return string(rbody), nil
	}
	return "", errors.New(resp.Status + ": " + string(rbody))
}

func NewClient(addr string) *Client {
	return &Client{baseURL: fmt.Sprintf("http://%s:%d", addr, WeaveHTTPPort)}
}

func NewClientWithResolver(resolver func() (string, error)) *Client {
	return &Client{resolve: resolver}
}

func (client *Client) Connect(remote string) error {
	_, err := client.httpVerb("POST", "/connect", url.Values{"peer": {remote}})
	return err
}
