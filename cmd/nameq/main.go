package main

import (
	"fmt"
	"os"
	"path"
)

func main() {
	prog := path.Base(os.Args[0])

	if len(os.Args) >= 2 {
		subprog := os.Args[1]
		command := prog + " " + subprog

		os.Args = os.Args[1:]

		switch subprog {
		case "feature":
			exit(command, feature(prog, command))

		case "monitor-features":
			exit(command, monitorFeatures(prog, command))

		case "find-features":
			exit(command, findFeatures(prog, command))

		case "serve":
			exit(command, serve(prog, command))
		}
	}

	fmt.Fprintf(os.Stderr, "Usage: %s feature [OPTIONS] NAME [VALUE]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s monitor-features [OPTIONS]\n", prog)
	fmt.Fprintf(os.Stderr, "       %s find-features [OPTIONS] NAME...\n", prog)
	fmt.Fprintf(os.Stderr, "       %s serve [OPTIONS]\n", prog)
	os.Exit(2)
}

func exit(command string, err error) {
	if err == nil {
		os.Exit(0)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %s\n", command, err)
		os.Exit(1)
	}
}
