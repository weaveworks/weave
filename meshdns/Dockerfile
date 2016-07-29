FROM golang:1.6

RUN apt-get update && apt-get install -y libpcap-dev git-core

RUN mkdir -p /go/src/github.com/weaveworks && git clone --depth=10 https://github.com/weaveworks/weave /go/src/github.com/weaveworks/weave
RUN mkdir -p /go/src/github.com/docker && git clone --depth=10 https://github.com/docker/docker /go/src/github.com/docker/docker

COPY . /go/src/github.com/weaveworks/weave/meshdns

WORKDIR /go/src/github.com/weaveworks/weave/meshdns

RUN go get -v -d

RUN go install -v

VOLUME /db

EXPOSE 53/udp 53/tcp 6789/udp 6789/tcp 8080/tcp

ENV PEERS=""

CMD /go/bin/meshdns --name=$(cat /sys/class/net/eth0/address) --db-prefix=/db --dns-listen-address=0.0.0.0:53 --resolv-conf=./resolv.conf --http-addr=0.0.0.0:8080 --port=6789 $PEERS
