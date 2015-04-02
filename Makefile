GO		:= go
export GOPATH	:= $(PWD)

nameq: $(wildcard cmd/*.go service/*.go)
	$(GO) get github.com/aarzilli/sandblast
	$(GO) get github.com/awslabs/aws-sdk-go/gen
	$(GO) get github.com/miekg/dns
	$(GO) get github.com/vaughan0/go-ini
	$(GO) get golang.org/x/exp/inotify
	$(GO) get golang.org/x/net/html

	$(GO) fmt cmd/*.go
	$(GO) fmt service/*.go
	$(GO) fmt go/*.go

	$(GO) vet cmd/*.go
	$(GO) vet service/*.go
	$(GO) vet go/*.go

	$(GO) build -o $@ ./cmd

check::
	$(GO) test -v ./service

clean:
	rm -f nameq
	rm -rf pkg
	rm -rf src

.PHONY: clean
