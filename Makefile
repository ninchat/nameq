GO		:= go
export GOPATH	:= $(PWD)

nameq: $(wildcard cmd/*.go service/*.go)
	$(GO) get github.com/aarzilli/sandblast
	$(GO) get github.com/aws/aws-sdk-go
	$(GO) get github.com/go-ini/ini
	$(GO) get github.com/jmespath/go-jmespath
	$(GO) get github.com/miekg/dns
	$(GO) get github.com/vaughan0/go-ini
	$(GO) get golang.org/x/exp/inotify
	$(GO) get golang.org/x/net/context
	$(GO) get golang.org/x/net/html

	$(GO) fmt cmd/*.go
	$(GO) fmt service/*.go
	$(GO) fmt go/*.go

	$(GO) vet cmd/*.go
	$(GO) vet service/*.go
	$(GO) vet go/*.go

	$(GO) build -o $@ ./cmd

check::
	$(GO) test -race -v ./go

clean:
	rm -f nameq
	rm -rf pkg
	rm -rf src/github.com/aarzilli/sandblast
	rm -rf src/github.com/aws/aws-sdk-go
	rm -rf src/github.com/go-ini/ini
	rm -rf src/github.com/jmespath/go-jmespath
	rm -rf src/github.com/miekg/dns
	rm -rf src/github.com/vaughan0/go-ini
	rm -rf src/golang.org/x/exp
	rm -rf src/golang.org/x/net
	rm -rf src/golang.org/x/text

.PHONY: clean
