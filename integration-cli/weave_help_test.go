package main

import (
        "fmt"
        "os/exec"
)

func ExampleF_weaveEmpty() {
        cmd := exec.Command(sudoBinary, weaveBinary)
        out, err := cmd.Output()
        if err != nil {
                fmt.Printf(err.Error())
        }
        fmt.Printf("%s", string(out))

        // Output:
        // Usage:
        // weave setup
        // weave launch     [-password <password>] [-nickname <nickname>] <peer> ...
        // weave launch-dns <cidr>
        // weave connect    <peer>
        // weave run        [--with-dns] <cidr> <docker run args> ...
        // weave start      <cidr> <container_id>
        // weave attach     <cidr> <container_id>
        // weave detach     <cidr> <container_id>
        // weave expose     <cidr>
        // weave hide       <cidr>
        // weave ps
        // weave status
        // weave version
        // weave stop
        // weave stop-dns
        // weave reset
        //
        // where <peer> is of the form <ip_address_or_fqdn>[:<port>], and
        //       <cidr> is of the form <ip_address>/<routing_prefix_length>
}
