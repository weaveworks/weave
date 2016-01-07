FROM golang:1.5.2
ENV GO15VENDOREXPERIMENT 1
RUN apt-get update && apt-get install -y libpcap-dev python-requests time
RUN go clean -i net && go install -tags netgo std
COPY build.sh /
ENTRYPOINT ["sh", "/build.sh"]
