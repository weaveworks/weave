FROM golang:1.7.3
RUN apt-get update && apt-get install -y libpcap-dev python-requests time file
RUN go get github.com/golang/lint/golint github.com/fzipp/gocyclo github.com/client9/misspell/cmd/misspell
RUN go clean -i net && go install -tags netgo std
RUN go install -race -tags netgo std
COPY build.sh /
ENTRYPOINT ["sh", "/build.sh"]
