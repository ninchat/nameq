package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"syscall"

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
		fmt.Fprintf(os.Stderr, "This command prints the current state, followed by updates in real time (until terminated with a signal or output pipe is closed).  The output lines are formatted like this (excluding quotes):\n\n")
		fmt.Fprintf(os.Stderr, "  \"NAME<tab>HOST<tab>STATE\"\n\n")
		fmt.Fprintf(os.Stderr, "NAME is a feature name.  HOST is the IPv4 or IPv6 address of a host where the feature exists.  STATE is either \"on\" or \"off\" (excluding quotes).  The JSON configurations of features are not available via this command.\n\n")
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

		if _, err := fmt.Printf("%s\t%s\t%s\n", f.Name, f.Host, state); err != nil {
			if pathErr, ok := err.(*os.PathError); ok {
				if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == syscall.EPIPE {
					os.Exit(0)
				}
			}

			logger.Print(err)
			os.Exit(1)
		}
	}
}
