package main

import (
	"fmt"
	"os"
	"path"
)

func main() {
	prog := path.Base(os.Args[0])

	if len(os.Args) >= 2 {
		sub := os.Args[1]
		subprog := prog + " " + sub

		os.Args = os.Args[1:]

		switch sub {
		case "name":
			exit(subprog, name(subprog))

		case "feature":
			exit(subprog, feature(subprog))

		case "monitor-features":
			exit(subprog, monitorFeatures(subprog))

		case "serve":
			exit(subprog, serve(subprog))
		}
	}

	fmt.Fprintf(os.Stderr, "Usage: %s name [OPTIONS] NAME\n", prog)
	fmt.Fprintf(os.Stderr, "       %s feature [OPTIONS] NAME [VALUE]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s monitor-features [OPTIONS]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s serve [OPTIONS]\n", prog)
	os.Exit(2)
}

func exit(subprog string, err error) {
	if err == nil {
		os.Exit(0)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %s\n", subprog, err)
		os.Exit(1)
	}
}
