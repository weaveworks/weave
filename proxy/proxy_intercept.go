package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

const (
	maxLineLength = 4096 // assumed <= bufio.defaultBufSize
	maxChunkSize  = bufio.MaxScanTokenSize
)

var (
	ErrChunkTooLong           = errors.New("chunk too long")
	ErrInvalidChunkLength     = errors.New("invalid byte in chunk length")
	ErrLineTooLong            = errors.New("header line too long")
	ErrMalformedChunkEncoding = errors.New("malformed chunked encoding")
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
	down, downBuf, up, remaining, err := hijack(w, client)
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
	go copyStream(down, io.MultiReader(remaining, up), upDone)
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

type writeFlusher interface {
	io.Writer
	http.Flusher
}

func doChunkedResponse(w http.ResponseWriter, resp *http.Response, client *httputil.ClientConn) {
	wf, ok := w.(writeFlusher)
	if !ok {
		http.Error(w, "Error forwarding chunked response body: flush not available", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)

	up, remaining := client.Hijack()
	defer up.Close()

	var err error
	chunks := bufio.NewScanner(io.MultiReader(remaining, up))
	chunks.Split(splitChunks)
	for chunks.Scan() && err == nil {
		_, err = wf.Write(chunks.Bytes())
		wf.Flush()
	}
	if err == nil {
		err = chunks.Err()
	}
	if err != nil {
		Log.Errorf("Error forwarding chunked response body: %s", err)
	}
}

// a bufio.SplitFunc for http chunks
func splitChunks(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		return 0, nil, nil
	}
	if i > maxLineLength {
		return 0, nil, ErrLineTooLong
	}

	chunkSize64, err := strconv.ParseInt(
		string(bytes.TrimRight(data[:i], " \t\r\n")),
		16,
		64,
	)
	switch {
	case err != nil:
		return 0, nil, ErrInvalidChunkLength
	case chunkSize64 > maxChunkSize:
		return 0, nil, ErrChunkTooLong
	case chunkSize64 == 0:
		return 0, nil, io.EOF
	}
	chunkSize := int(chunkSize64)

	data = data[i+1:]

	if len(data) < chunkSize+2 {
		return 0, nil, nil
	}

	if data[chunkSize] != '\r' || data[chunkSize+1] != '\n' {
		return 0, nil, ErrMalformedChunkEncoding
	}

	return i + chunkSize + 3, data[:chunkSize], nil
}

func hijack(w http.ResponseWriter, client *httputil.ClientConn) (down net.Conn, downBuf *bufio.ReadWriter, up net.Conn, remaining io.Reader, err error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		err = errors.New("Unable to cast to Hijack")
		return
	}
	down, downBuf, err = hj.Hijack()
	if err != nil {
		return
	}
	up, remaining = client.Hijack()
	return
}
