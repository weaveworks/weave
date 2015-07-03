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

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

func (proxy *Proxy) Intercept(i interceptor, w http.ResponseWriter, r *http.Request) {
	if err := i.InterceptRequest(r); err != nil {
		switch err.(type) {
		case *docker.NoSuchContainer:
			http.Error(w, err.Error(), http.StatusNotFound)
		case *ErrNoSuchImage:
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			Log.Warning("Error intercepting request: ", err)
		}
		return
	}

	conn, err := proxy.Dial()
	if err != nil {
		http.Error(w, "Could not connect to target", http.StatusInternalServerError)
		Log.Warning(err)
		return
	}
	client := httputil.NewClientConn(conn, nil)
	defer client.Close()

	resp, err := client.Do(r)
	if err != nil && err != httputil.ErrPersistEOF {
		http.Error(w, fmt.Sprintf("Could not make request to target: %v", err), http.StatusInternalServerError)
		Log.Warning("Error forwarding request: ", err)
		return
	}
	err = i.InterceptResponse(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		Log.Warning("Error intercepting response: ", err)
		return
	}

	hdr := w.Header()
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	Log.Debugf("Response from target: %s %v", resp.Status, w.Header())

	if resp.Header.Get("Content-Type") == "application/vnd.docker.raw-stream" {
		doRawStream(w, resp, client)
	} else if resp.TransferEncoding != nil && resp.TransferEncoding[0] == "chunked" {
		doChunkedResponse(w, resp, client)
	} else {
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			Log.Warning(err)
		}
	}
}

func doRawStream(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	down, downBuf, up, rem, err := hijack(w, client)
	if err != nil {
		http.Error(w, "Unable to hijack connection for raw stream mode", http.StatusInternalServerError)
		return
	}
	defer down.Close()
	defer up.Close()

	if _, err := down.Write([]byte("HTTP/1.1 200 OK\n")); err != nil {
		Log.Warning(err)
		return
	}

	if err := resp.Header.Write(down); err != nil {
		Log.Warning(err)
		return
	}

	if _, err := down.Write([]byte("\n")); err != nil {
		Log.Warning(err)
		return
	}

	upDone := make(chan struct{})
	downDone := make(chan struct{})
	go copyStream(down, io.MultiReader(rem, up), upDone)
	go copyStream(up, downBuf, downDone)
	<-upDone
	<-downDone
}

func copyStream(dst io.Writer, src io.Reader, done chan struct{}) {
	defer close(done)
	if _, err := io.Copy(dst, src); err != nil {
		Log.Warning(err)
	}
	if c, ok := dst.(interface {
		CloseWrite() error
	}); ok {
		if err := c.CloseWrite(); err != nil {
			Log.Warningf("Error closing connection: %s", err)
		}
	}
}

func doChunkedResponse(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	// Because we can't go back to request/response after we
	// hijack the connection, we need to close it and make the
	// client open another.
	w.Header().Add("Connection", "close")
	w.WriteHeader(resp.StatusCode)

	down, _, up, rem, err := hijack(w, client)
	if err != nil {
		http.Error(w, "Unable to hijack response stream for chunked response", http.StatusInternalServerError)
		return
	}
	defer up.Close()
	defer down.Close()
	// Copy the chunked response body to downstream,
	// stopping at the end of the chunked section.
	rawResponseBody := io.MultiReader(rem, up)
	if _, err := io.Copy(ioutil.Discard, httputil.NewChunkedReader(io.TeeReader(rawResponseBody, down))); err != nil {
		http.Error(w, "Error copying chunked response body", http.StatusInternalServerError)
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
