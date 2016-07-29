FROM golang:1.6

RUN apt-get update && apt-get install -y libpcap-dev

COPY . /go/src/github.com/weaveworks/meshdns

WORKDIR /go/src/github.com/weaveworks/meshdns

RUN go get -v -d

RUN go install -v
