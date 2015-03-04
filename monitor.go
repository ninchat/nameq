package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	nameq "./go"
)

var (
	localHost = net.IPv4(127, 0, 0, 1)
)

func monitorFeatures() {
	var (
		stateDir = nameq.DefaultStateDir
		local    = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", subprog)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
	}

	flag.StringVar(&stateDir, "statedir", stateDir, "runtime state root location")
	flag.BoolVar(&local, "local", local, "monitor only features on the localhost")

	flag.Parse()

	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}

	logger := log.New(os.Stderr, subprog+": ", 0)

	monitor, err := nameq.NewFeatureMonitor(stateDir, logger)
	if err != nil {
		logger.Print(err)
		os.Exit(1)
	}

	for f := range monitor.C {
		if local && !f.Host.Equal(localHost) {
			continue
		}

		var state string

		if f.Data != nil {
			state = "on"
		} else {
			state = "off"
		}

		fmt.Printf("%s\t%s\t%s\n", f.Name, f.Host, state)
	}
}
