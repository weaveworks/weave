package weavedns

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

func ListenHttp(db Zone, port int) {
	http.HandleFunc("/add", func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		addr := r.FormValue("ip")
		weave_cidr := r.FormValue("weave_cidr")
		if name != "" && addr != "" && weave_cidr != "" {
			ip := net.ParseIP(addr)
			weave_ip, subnet, err := net.ParseCIDR(weave_cidr)
			if err == nil && ip != nil {
				log.Printf("Adding %s (%s) -> %s", name, addr, weave_cidr)
				db.AddRecord(name, ip, weave_ip, subnet)
			} else if err != nil {
				log.Printf("Invalid CIDR in request: %s", weave_cidr)
			} else {
				log.Printf("Invalid IP in request: %s", addr)
			}
		} else {
			log.Printf("Invalid request: %s, %s, %s", name, addr, weave_cidr)
		}
	})
	address := fmt.Sprintf(":%d", port)
	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatal("Unable to create http listener: ", err)
	}
}
