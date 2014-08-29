package weave

import (
	"fmt"
    "net/http"
)

/* A simple http responder that will show the status of the router when asked,
 e.g. via http://172.17.0.2/status
*/
type HttpStatusResponder struct {
    router *Router
}

func NewHttpStatusResponder(router *Router) *HttpStatusResponder {
    resp := &HttpStatusResponder{router}
    return resp
}

func (resp *HttpStatusResponder) SetRouter(newRouter *Router) {
    resp.router = newRouter
}

func (resp *HttpStatusResponder) ListenAndServe() {
    http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
        if resp.router != nil {
            fmt.Fprint(w, resp.router.Status())
        } else {
            fmt.Fprintf(w, "Not initialized")
        }
    })
    address := fmt.Sprintf(":%d", StatusPort)
    http.ListenAndServe(address, nil)
}
