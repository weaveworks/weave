package skel

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/docker/libnetwork/drivers/remote/api"
)

const (
	MethodReceiver = "NetworkDriver"
)

type Driver interface {
	GetCapabilities() (*api.GetCapabilityResponse, error)
	CreateNetwork(create *api.CreateNetworkRequest) error
	DeleteNetwork(delete *api.DeleteNetworkRequest) error
	CreateEndpoint(create *api.CreateEndpointRequest) (*api.CreateEndpointResponse, error)
	DeleteEndpoint(delete *api.DeleteEndpointRequest) error
	EndpointInfo(req *api.EndpointInfoRequest) (*api.EndpointInfoResponse, error)
	JoinEndpoint(j *api.JoinRequest) (response *api.JoinResponse, error error)
	LeaveEndpoint(leave *api.LeaveRequest) error
	DiscoverNew(discover *api.DiscoveryNotification) error
	DiscoverDelete(delete *api.DiscoveryNotification) error
}

type listener struct {
	d Driver
}

func Listen(socket net.Listener, driver Driver) error {
	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(notFound)

	router.Methods("POST").Path("/Plugin.Activate").HandlerFunc(handshake)

	handleMethod := func(method string, h http.HandlerFunc) {
		router.Methods("POST").Path(fmt.Sprintf("/%s.%s", MethodReceiver, method)).HandlerFunc(h)
	}

	listener := &listener{driver}

	handleMethod("GetCapabilities", listener.getCapabilities)
	handleMethod("CreateNetwork", listener.createNetwork)
	handleMethod("DeleteNetwork", listener.deleteNetwork)
	handleMethod("CreateEndpoint", listener.createEndpoint)
	handleMethod("DeleteEndpoint", listener.deleteEndpoint)
	handleMethod("EndpointOperInfo", listener.infoEndpoint)
	handleMethod("Join", listener.joinEndpoint)
	handleMethod("Leave", listener.leaveEndpoint)

	return http.Serve(socket, router)
}

// === protocol handlers

type handshakeResp struct {
	Implements []string
}

func handshake(w http.ResponseWriter, r *http.Request) {
	err := json.NewEncoder(w).Encode(&handshakeResp{
		[]string{"NetworkDriver"},
	})
	if err != nil {
		sendError(w, "encode error", http.StatusInternalServerError)
		return
	}
}

func (listener *listener) getCapabilities(w http.ResponseWriter, r *http.Request) {
	caps, err := listener.d.GetCapabilities()
	objectOrErrorResponse(w, caps, err)
}

func (listener *listener) createNetwork(w http.ResponseWriter, r *http.Request) {
	var create api.CreateNetworkRequest
	err := json.NewDecoder(r.Body).Decode(&create)
	if err != nil {
		sendError(w, "Unable to decode JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	emptyOrErrorResponse(w, listener.d.CreateNetwork(&create))
}

func (listener *listener) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	var delete api.DeleteNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&delete); err != nil {
		sendError(w, "Unable to decode JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	emptyOrErrorResponse(w, listener.d.DeleteNetwork(&delete))
}

func (listener *listener) createEndpoint(w http.ResponseWriter, r *http.Request) {
	var create api.CreateEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		sendError(w, "unable to decode JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	res, err := listener.d.CreateEndpoint(&create)
	objectOrErrorResponse(w, res, err)
}

func (listener *listener) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	var delete api.DeleteEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&delete); err != nil {
		sendError(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	emptyOrErrorResponse(w, listener.d.DeleteEndpoint(&delete))
}

func (listener *listener) infoEndpoint(w http.ResponseWriter, r *http.Request) {
	var req api.EndpointInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	info, err := listener.d.EndpointInfo(&req)
	objectOrErrorResponse(w, info, err)
}

func (listener *listener) joinEndpoint(w http.ResponseWriter, r *http.Request) {
	var join api.JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&join); err != nil {
		sendError(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	res, err := listener.d.JoinEndpoint(&join)
	objectOrErrorResponse(w, res, err)
}

func (listener *listener) leaveEndpoint(w http.ResponseWriter, r *http.Request) {
	var l api.LeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		sendError(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	emptyOrErrorResponse(w, listener.d.LeaveEndpoint(&l))
}

func (listener *listener) discoverNew(w http.ResponseWriter, r *http.Request) {
	var disco api.DiscoveryNotification
	if err := json.NewDecoder(r.Body).Decode(&disco); err != nil {
		sendError(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	emptyOrErrorResponse(w, listener.d.DiscoverNew(&disco))
}

func (listener *listener) discoverDelete(w http.ResponseWriter, r *http.Request) {
	var disco api.DiscoveryNotification
	if err := json.NewDecoder(r.Body).Decode(&disco); err != nil {
		sendError(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	emptyOrErrorResponse(w, listener.d.DiscoverDelete(&disco))
}

// ===

func notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func sendError(w http.ResponseWriter, msg string, code int) {
	http.Error(w, msg, code)
}

func errorResponse(w http.ResponseWriter, fmtString string, item ...interface{}) {
	json.NewEncoder(w).Encode(map[string]string{
		"Err": fmt.Sprintf(fmtString, item...),
	})
}

func objectResponse(w http.ResponseWriter, obj interface{}) {
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		sendError(w, "Could not JSON encode response", http.StatusInternalServerError)
		return
	}
}

func emptyResponse(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(map[string]string{})
}

func objectOrErrorResponse(w http.ResponseWriter, obj interface{}, err error) {
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	objectResponse(w, obj)
}

func emptyOrErrorResponse(w http.ResponseWriter, err error) {
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	emptyResponse(w)
}
