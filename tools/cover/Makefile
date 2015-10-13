.PHONY: all clean

all: cover

cover: *.go
	go get -tags netgo ./$(@D)
	go build -ldflags "-extldflags \"-static\" -linkmode=external" -tags netgo -o $@ ./$(@D)

clean:
	rm -rf cover
	go clean ./...
