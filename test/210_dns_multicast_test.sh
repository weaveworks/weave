#! /bin/bash

. ./config.sh

C1=10.2.3.78
C2=10.2.3.34
C2a=10.2.3.35
C2b=10.2.3.36
UNIVERSE=10.2.4.0/24
NAME2=seetwo.weave.local
NAME4=seefour.weave.local
DNS1=10.2.254.1
DNS2=10.2.254.2

start() {
    launch_dns_on $HOST1 $1 >/dev/null
    launch_dns_on $HOST2 $2 >/dev/null
}

stop() {
    weave_on $HOST1 stop-dns
    weave_on $HOST2 stop-dns
}

check() {
    start $1 $2
    assert_dns_record $HOST1 c1 $NAME2 $C2
    assert_raises "exec_on $HOST1 c1 getent hosts 8.8.8.8 | grep google"
    stop
}

start_suite "Resolve names across hosts (with and without IPAM), and repopulate on restart"

weave_on $HOST1 launch-router --ipalloc-range $UNIVERSE
weave_on $HOST2 launch-router --ipalloc-range $UNIVERSE $HOST1

start_container          $HOST2 $C2/24 --name=c2 -h $NAME2
start_container_with_dns $HOST1 $C1/24 --name=c1

# resolution when DNS is launched with various ways of obtaining an IP
# NB: this also tests re-population on DNS restart
check  ip:$DNS1/24      ip:$DNS2/24
check net:10.2.4.128/25 net:10.2.4.128/25
check net:default       net:default
check

start $DNS1/24 $DNS2/24

# resolution for names mapped to multiple addresses
weave_on $HOST2 dns-add $C2a c2 -h $NAME2
weave_on $HOST2 dns-add $C2b c2 -h $NAME2
assert_dns_record $HOST1 c1 $NAME2 $C2 $C2a $C2b

# resolution when containers addresses come from IPAM
start_container          $HOST2 --name=c4 -h $NAME4
start_container_with_dns $HOST1 --name=c3
C4=$(container_ip $HOST2 c4)
assert_dns_record $HOST1 c3 $NAME4 $C4
assert_raises "exec_on $HOST1 c3 getent hosts 8.8.8.8 | grep google"

# check that dns.weave.local returns the local WeaveDNS (and not remote instances)
start_container_with_dns $HOST2 --name=c5
assert_dns_record $HOST1 c1 "dns.weave.local" $DNS1
assert_dns_record $HOST2 c5 "dns.weave.local" $DNS2

stop

end_suite
