package weavedns

import (
	"fmt"
	"log"
	"net/http"
)

const (
	UPDATE_PORT = 6785
)

func ListenHttp(db Zone) {
	http.HandleFunc("/add", func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		addr := r.FormValue("ip")
		weave_ip := r.FormValue("weave_ip")
		log.Printf("Adding %s %s %s", name, addr, weave_ip)
		db.AddRecord(name, addr, weave_ip)
	})
	address := fmt.Sprintf(":%d", UPDATE_PORT)
	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatal("Unable to create http listener: ", err)
	}
}
