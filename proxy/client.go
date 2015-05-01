package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"

	. "github.com/weaveworks/weave/common"
)

type client struct {
	interceptor
	Dial func() (net.Conn, error)
}

func newClient(dial func() (net.Conn, error), i interceptor) *client {
	return &client{
		interceptor: i,
		Dial:        dial,
	}
}

func (c *client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := c.InterceptRequest(r)
	if err == docker.ErrNoSuchImage {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Unable to create proxied request", http.StatusInternalServerError)
		Warning.Print("Error intercepting request: ", err)
		return
	}

	conn, err := c.Dial()
	if err != nil {
		http.Error(w, "Could not connect to target", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}
	client := httputil.NewClientConn(conn, nil)
	defer client.Close()

	resp, err := client.Do(req)
	if err != nil && err != httputil.ErrPersistEOF {
		http.Error(w, fmt.Sprintf("Could not make request to target: %s", err.Error()), http.StatusInternalServerError)
		Warning.Print("Error forwarding request: ", err)
		return
	}
	resp, err = c.InterceptResponse(resp)
	if err != nil {
		http.Error(w, "Unable to intercept response", http.StatusInternalServerError)
		Warning.Print("Error intercepting response: ", err)
		return
	}

	hdr := w.Header()
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	Debug.Printf("Response from target: %s %v", resp.Status, w.Header())

	if resp.Header.Get("Content-Type") == "application/vnd.docker.raw-stream" {
		doRawStream(w, resp, client)
	} else if resp.TransferEncoding != nil && resp.TransferEncoding[0] == "chunked" {
		doChunkedResponse(w, resp, client)
	} else {
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			Warning.Print(err)
		}
	}
}

func doRawStream(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	down, downBuf, up, rem, err := hijack(w, client)
	if err != nil {
		Error.Fatal(w, "Unable to hijack connection for raw stream mode", http.StatusInternalServerError)
		return
	}
	defer down.Close()
	defer up.Close()

	if _, err := down.Write([]byte("HTTP/1.1 200 OK\n")); err != nil {
		Warning.Print(err)
		return
	}

	if err := resp.Header.Write(down); err != nil {
		Warning.Print(err)
		return
	}

	if _, err := down.Write([]byte("\n")); err != nil {
		Warning.Print(err)
		return
	}

	end := make(chan bool)
	go func() {
		defer close(end)
		if _, err := io.Copy(up, downBuf); err != nil {
			Warning.Print(err)
		}
		err = up.(interface {
			CloseWrite() error
		}).CloseWrite()
		if err != nil {
			Debug.Printf("Error Closing upstream: %s", err)
		}
	}()

	if _, err := io.Copy(down, io.MultiReader(rem, up)); err != nil {
		Warning.Print(err)
	}
	err = down.(interface {
		CloseWrite() error
	}).CloseWrite()
	if err != nil {
		Debug.Printf("Error Closing downstream: %s", err)
	}

	<-end
}

func doChunkedResponse(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	// Because we can't go back to request/response after we
	// hijack the connection, we need to close it and make the
	// client open another.
	w.Header().Add("Connection", "close")
	w.WriteHeader(resp.StatusCode)

	down, _, up, rem, err := hijack(w, client)
	if err != nil {
		Error.Fatal("Unable to hijack response stream for chunked response", http.StatusInternalServerError)
		return
	}
	defer up.Close()
	defer down.Close()
	// Copy the chunked response body to downstream,
	// stopping at the end of the chunked section.
	rawResponseBody := io.MultiReader(rem, up)
	if _, err := io.Copy(ioutil.Discard, httputil.NewChunkedReader(io.TeeReader(rawResponseBody, down))); err != nil {
		Error.Fatal("Error copying chunked response body", http.StatusInternalServerError)
		return
	}
	resp.Trailer.Write(down)
	// a chunked response ends with a CRLF
	down.Write([]byte("\r\n"))
}

func hijack(w http.ResponseWriter, client *httputil.ClientConn) (down net.Conn, downBuf *bufio.ReadWriter, up net.Conn, rem io.Reader, err error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		err = errors.New("Unable to cast to Hijack")
		return
	}
	down, downBuf, err = hj.Hijack()
	if err != nil {
		return
	}
	up, rem = client.Hijack()
	return
}
