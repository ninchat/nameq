package main

import (
	"fmt"
	"os"
	"path"
)

var (
	subprog string
)

func main() {
	prog := path.Base(os.Args[0])

	if len(os.Args) >= 2 {
		sub := os.Args[1]
		subprog = prog + " " + sub

		os.Args = os.Args[1:]

		switch sub {
		case "name":
			name()
			return

		case "feature":
			feature()
			return

		case "monitor-features":
			monitorFeatures()
			return

		case "serve":
			serve()
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Usage: %s name [OPTIONS] NAME\n", prog)
	fmt.Fprintf(os.Stderr, "       %s feature [OPTIONS] NAME [VALUE]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s monitor-features [OPTIONS]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s serve [OPTIONS]\n", prog)
	os.Exit(2)
}
