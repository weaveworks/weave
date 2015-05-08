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
	"net/url"

	. "github.com/weaveworks/weave/common"
)

const (
	RAW_STREAM = "application/vnd.docker.raw-stream"
)

type proxy struct {
	Dial func() (net.Conn, error)
	mux  *http.ServeMux
}

func targetNetwork(u *url.URL) string {
	return u.Scheme
}

func targetAddress(u *url.URL) (addr string) {
	switch u.Scheme {
	case "tcp":
		addr = u.Host
	case "unix":
		addr = u.Path
	}
	return
}

func NewProxy(targetUrl string) (*proxy, error) {
	u, err := url.Parse(targetUrl)
	if err != nil {
		return nil, err
	}
	p := &proxy{
		Dial: func() (net.Conn, error) {
			return net.Dial(targetNetwork(u), targetAddress(u))
		},
	}
	p.setupMux()
	return p, nil
}

func (proxy *proxy) setupMux() {
	proxy.mux = http.NewServeMux()
	proxy.mux.HandleFunc("/weave", proxy.WeaveStatus)
	proxy.mux.HandleFunc("/", proxy.ProxyRequest)
}

func (proxy *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	proxy.mux.ServeHTTP(w, r)
}

func (proxy *proxy) WeaveStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (proxy *proxy) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	req, err := proxy.InterceptRequest(r)
	if err != nil {
		http.Error(w, "Unable to create proxied request", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}

	conn, err := proxy.Dial()
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
		Warning.Print(err)
		return
	}
	resp = proxy.InterceptResponse(req, resp)

	hdr := w.Header()
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	Debug.Printf("Response from target: %s %v", resp.Status, w.Header())

	if resp.Header.Get("Content-Type") == RAW_STREAM {
		doRawStream(w, resp, client)
	} else if resp.TransferEncoding != nil &&
		resp.TransferEncoding[0] == "chunked" {
		doChunkedResponse(w, resp, client)
	} else {
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			Warning.Print(err)
		}
	}
}

// Supplied so that we can use with http.Client as a Transport
func (proxy *proxy) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := &http.Transport{
		Dial: func(string, string) (conn net.Conn, err error) {
			conn, err = proxy.Dial()
			return
		},
	}
	res, err := transport.RoundTrip(req)
	return res, err
}

func doRawStream(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	down, downBuf, up, rem, err := hijack(w, client)
	defer down.Close()
	defer up.Close()

	if err != nil {
		Error.Fatal(w, "Unable to hijack connection for raw stream mode", http.StatusInternalServerError)
		return
	}

	end := make(chan bool)

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

	go func() {
		defer close(end)

		if _, err := io.Copy(down, io.MultiReader(rem, up)); err != nil {
			Warning.Print(err)
		}
	}()
	go func() {
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
	<-end
}

func doChunkedResponse(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	// Because we can't go back to request/response after we
	// hijack the connection, we need to close it and make the
	// client open another.
	w.Header().Add("Connection", "close")
	w.WriteHeader(resp.StatusCode)

	down, _, up, rem, err := hijack(w, client)
	defer up.Close()
	defer down.Close()
	if err != nil {
		Error.Fatal("Unable to hijack response stream for chunked response", http.StatusInternalServerError)
		return
	}
	// Copy the chunked response body to downstream,
	// stopping at the end of the chunked section.
	rawResponseBody := io.MultiReader(rem, up)
	if _, err := io.Copy(ioutil.Discard, httputil.NewChunkedReader(io.TeeReader(rawResponseBody, down))); err != nil {
		Error.Fatal("Error copying chunked response body", http.StatusInternalServerError)
		return
	}
	resp.Trailer.Write(down)
	// a chunked response ends with a CRLF
	down.Write([]byte{13, 10})
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
