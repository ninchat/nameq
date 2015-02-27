GO		:= go
export GOPATH	:= $(PWD)

nameq: $(wildcard *.go service/*.go)
	$(GO) get github.com/aarzilli/sandblast
	$(GO) get github.com/awslabs/aws-sdk-go/gen
	$(GO) get github.com/miekg/dns
	$(GO) get github.com/vaughan0/go-ini
	$(GO) get golang.org/x/exp/inotify
	$(GO) get golang.org/x/net/html

	$(GO) fmt *.go
	$(GO) fmt ./service
	$(GO) fmt ./go

	$(GO) vet *.go
	$(GO) vet ./service
	$(GO) vet ./go

	$(GO) build -o $@

clean:
	rm -f nameq
	rm -rf pkg
	rm -rf src

.PHONY: clean
