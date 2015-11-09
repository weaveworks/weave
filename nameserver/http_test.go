package nameserver

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"

	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

func randRange(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func TestHTTPWebSocket(t *testing.T) {
	const (
		name   = "some_host.weave.local."
		origin = "http://localhost"
	)

	var (
		port  = randRange(18080, 28080)
		wsURL = fmt.Sprintf("ws://127.0.0.1:%d/name/ws", port)
	)

	// create the DNS server
	peername, err := router.PeerNameFromString("00:00:00:02:00:00")
	require.Nil(t, err)
	nameserver := New(peername, "", func(router.PeerName) bool { return true })

	// ... and the web server
	muxRouter := mux.NewRouter()
	nameserver.HandleHTTP(muxRouter, nil)
	httpServer := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: muxRouter}
	go func() {
		err := httpServer.ListenAndServe()
		require.Nil(t, err)
	}()

	time.Sleep(1 * time.Second) // give some time to the webserver to start up...

	ws, err := websocket.Dial(wsURL, "", origin)
	require.Nil(t, err)

	waitForUpdates := func() []address.Address {
		var msg = make([]byte, 512)
		l, err := ws.Read(msg)
		require.Nil(t, err)

		var update struct {
			Addresses []string
		}
		err = json.Unmarshal(msg[:l], &update)
		if err == io.EOF {
			ws.WriteClose(0)
			return nil
		}
		require.Nil(t, err)

		// convert the strings to address.Address
		res := []address.Address{}
		for _, v := range update.Addresses {
			ipStr, err := address.ParseIP(v)
			require.Nil(t, err)
			res = append(res, ipStr)
		}
		return res
	}

	addr1 := address.Address(123456)
	addr2 := address.Address(776688)

	checkAddresses := func(have, expected []address.Address) {
		if !reflect.DeepEqual(have, expected) {
			t.Fail()
		}
	}

	// Add an entry
	err = nameserver.AddEntry(name, "containerid", peername, addr1)
	require.Nil(t, err)

	// ... send a "observe" request and get the current IPs
	err = websocket.JSON.Send(ws, ObserveRequest{Name: name})
	require.Nil(t, err)
	have := waitForUpdates()
	checkAddresses(have, []address.Address{addr1})

	// ... add another IP to the same name
	err = nameserver.AddEntry(name, "containerid", peername, addr2)
	require.Nil(t, err)
	have = waitForUpdates()
	checkAddresses(have, []address.Address{addr1, addr2})

	// ... and then we remove the entry
	nameserver.ContainerDied("containerid")
	require.Equal(t, []address.Address{}, nameserver.Lookup(name))
	have = waitForUpdates()
	checkAddresses(have, []address.Address{})
}
