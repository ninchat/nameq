GO		:= go
export GOPATH	:= $(PWD)

nameq: $(wildcard *.go)
	$(GO) get github.com/aarzilli/sandblast
	$(GO) get github.com/awslabs/aws-sdk-go/gen
	$(GO) get github.com/miekg/dns
	$(GO) get github.com/vaughan0/go-ini
	$(GO) get golang.org/x/exp/inotify
	$(GO) get golang.org/x/net/html
	$(GO) fmt
	$(GO) vet
	$(GO) build -o $@

clean:
	rm -f nameq
	rm -rf pkg
	rm -rf src

.PHONY: clean
