package router

import (
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"strings"
)

func errorMessages(errors []error) string {
	var result []string
	for _, err := range errors {
		result = append(result, err.Error())
	}
	return strings.Join(result, "\n")
}

func (router *Router) HandleHTTP(muxRouter *mux.Router) {

	muxRouter.Methods("POST").Path("/connect").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprint("unable to parse form: ", err), http.StatusBadRequest)
		}
		if errors := router.ConnectionMaker.InitiateConnections(r.Form["peer"], r.FormValue("replace") == "true"); len(errors) > 0 {
			http.Error(w, errorMessages(errors), http.StatusBadRequest)
		}
	})

	muxRouter.Methods("POST").Path("/forget").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprint("unable to parse form: ", err), http.StatusBadRequest)
		}
		router.ConnectionMaker.ForgetConnections(r.Form["peer"])
	})

}
