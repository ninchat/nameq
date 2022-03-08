package main

import (
	"os"
	"path"

	"github.com/ninchat/nameq/cmd/nameq/command"
)

func main() {
	os.Args[0] = path.Base(os.Args[0])
	command.Main()
}
