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
			configureName()
			return

		case "feature":
			configureFeature()
			return

		case "monitor-features":
			monitorFeatures()
			return

		case "service":
			serve()
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Usage: %s name [OPTIONS] NAME\n", prog)
	fmt.Fprintf(os.Stderr, "       %s feature [OPTIONS] NAME [VALUE]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s monitor-features [OPTIONS]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s service [OPTIONS]\n", prog)
	os.Exit(2)
}
