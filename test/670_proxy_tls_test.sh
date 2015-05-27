#! /bin/bash

. ./config.sh

start_suite "Boot the proxy with TLS-enabled Docker support"

PWD=$($SSH $HOST1 pwd)

weave_on $HOST1 launch
weave_on $HOST1 launch-proxy \
  --tls \
  --tlscacert $PWD/tls/$HOST1.ca.pem \
  --tlscert   $PWD/tls/$HOST1.server.pem \
  --tlskey    $PWD/tls/server-key.pem

assert_raises "proxy docker_on $HOST1 --tls --tlscacert ./tls/$HOST1.ca.pem ps"

end_suite
