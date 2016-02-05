GO		:= go
export GOPATH	:= $(PWD)

build: nameq

nameq: $(wildcard cmd/*.go service/*.go)
	$(GO) get github.com/aws/aws-sdk-go
	$(GO) get github.com/aws/aws-sdk-go/aws/credentials
	$(GO) get github.com/aws/aws-sdk-go/aws/session
	$(GO) get github.com/aws/aws-sdk-go/service/s3
	$(GO) get golang.org/x/exp/inotify
	$(GO) get golang.org/x/net/context
	$(GO) fmt ./cmd ./service ./go
	$(GO) vet ./cmd ./service ./go
	$(GO) build -o $@ ./cmd

check:
	$(GO) test -race -v ./go

clean:
	rm -f nameq
	rm -rf pkg
	rm -rf src/github.com/aws
	rm -rf src/github.com/go-ini
	rm -rf src/github.com/jmespath
	rm -rf src/golang.org/x

.PHONY: build check clean
