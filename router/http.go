package router

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"github.com/rajch/weave/common"
	"github.com/rajch/weave/net"
	"github.com/rajch/weave/net/address"
)

func (router *NetworkRouter) HandleHTTP(muxRouter *mux.Router) {

	muxRouter.Methods("POST").Path("/connect").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprint("unable to parse form: ", err), http.StatusBadRequest)
		}
		if errors := router.InitiateConnections(r.Form["peer"], r.FormValue("replace") == "true"); len(errors) > 0 {
			http.Error(w, common.ErrorMessages(errors), http.StatusBadRequest)
		}
	})

	muxRouter.Methods("POST").Path("/forget").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprint("unable to parse form: ", err), http.StatusBadRequest)
		}
		router.ForgetConnections(r.Form["peer"])
	})

	muxRouter.Methods("POST").Path("/expose/{ip}/{prefixlen}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		cidr, err := address.ParseCIDR(vars["ip"] + "/" + vars["prefixlen"])
		if err != nil {
			http.Error(w, fmt.Sprint("unable to parse ip addr: ", err.Error()), http.StatusBadRequest)
			return
		}

		var skipNAT bool
		if r.FormValue("skipNAT") != "" {
			if skipNAT, err = strconv.ParseBool(r.FormValue("skipNAT")); err != nil {
				http.Error(w, fmt.Sprint("unable to parse skipNAT option: ", err.Error()), http.StatusBadRequest)
			}
		}

		if err = net.Expose(router.BridgeConfig.WeaveBridgeName, cidr.IPNet(), router.BridgeConfig.AWSVPC, router.BridgeConfig.NPC, skipNAT); err != nil {
			http.Error(w, fmt.Sprint("unable to expose: ", err.Error()), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(204)
	})

}
