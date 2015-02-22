GO		:= go
export GOPATH	:= $(PWD)

namep: namep.go
	$(GO) get github.com/miekg/dns
	$(GO) build $^

clean:
	rm -f namep
	rm -rf build
	rm -rf pkg
	rm -rf src

.PHONY: clean
