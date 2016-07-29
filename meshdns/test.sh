#!/bin/bash -uex

docker build -t weaveworks/standalone-dns .

docker-compose up -d
docker-compose scale peer=10

curl -XPUT "localhost:8080/name/xxx1/10.10.1.0?fqdn=test1.weave.local"
curl -XPUT "localhost:8080/name/xxx1/10.10.2.0?fqdn=test1.weave.local"
curl -XPUT "localhost:8080/name/xxx1/10.10.3.0?fqdn=test1.weave.local"
curl -XPUT "localhost:8080/name/xxx1/10.10.4.0?fqdn=test1.weave.local"


for c in $(docker-compose ps -q) ; do
  docker exec "${c}" curl --silent --fail "localhost:8080/status/dns"
done
