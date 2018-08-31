package router

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/net/address"
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

	muxRouter.Methods("POST").Path("/expose/{ip}/{prefixlen}/{skipNAT}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		var skipNAT bool
		cidr, err := address.ParseCIDR(vars["ip"] + "/" + vars["prefixlen"])
		if err != nil {
			http.Error(w, fmt.Sprint("unable to parse ip addr: ", err.Error()), http.StatusBadRequest)
			return
		}

		if vars["skipNAT"] != "" {
			skipNAT, err = strconv.ParseBool(vars["skipNAT"])
			if err != nil {
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
