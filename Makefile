GO		:= go
export GOPATH	:= $(PWD)

nameq: $(wildcard cmd/*.go service/*.go)
	$(GO) get github.com/aarzilli/sandblast
	$(GO) get github.com/miekg/dns
	$(GO) get github.com/vaughan0/go-ini
	$(GO) get golang.org/x/exp/inotify
	$(GO) get golang.org/x/net/html

	$(GO) build github.com/awslabs/aws-sdk-go/aws
	$(GO) build github.com/awslabs/aws-sdk-go/service/s3

	$(GO) fmt cmd/*.go
	$(GO) fmt service/*.go
	$(GO) fmt go/*.go

	$(GO) vet cmd/*.go
	$(GO) vet service/*.go
	$(GO) vet go/*.go

	$(GO) build -o $@ ./cmd

check::
	$(GO) test -v ./go/test

clean:
	rm -f nameq
	rm -rf pkg
	rm -rf src/github.com/aarzilli/sandblast
	rm -rf src/github.com/miekg/dns
	rm -rf src/github.com/vaughan0/go-ini
	rm -rf src/golang.org/x/exp
	rm -rf src/golang.org/x/net
	rm -rf src/golang.org/x/text

.PHONY: clean
