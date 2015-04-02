package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"unicode"

	nameq "../go"
)

var (
	localHost = net.IPv4(127, 0, 0, 1)
)

func monitorFeatures(prog, command string) (err error) {
	var (
		stateDir = nameq.DefaultStateDir
		local    = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", command)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "This command prints the current state, followed by updates in real time (until terminated with a signal or output pipe is closed).  The output lines are formatted like this (excluding quotes):\n\n")
		fmt.Fprintf(os.Stderr, "  \"NAME<tab>HOST<tab>VALUE\"\n\n")
		fmt.Fprintf(os.Stderr, "NAME is a feature name.  HOST is the IPv4 or IPv6 address of a host where the feature exists.  VALUE is a JSON document if feature was added or updated, or empty if feature was removed.  The JSON encoding doesn't use whitespace characters.\n\n")
	}

	flag.StringVar(&stateDir, "statedir", stateDir, "runtime state root location")
	flag.BoolVar(&local, "local", local, "monitor only features on the localhost")

	flag.Parse()

	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}

	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		os.Exit(0)
	}()

	logger := log.New(os.Stderr, prog+": ", 0)

	monitor, err := nameq.NewFeatureMonitor(stateDir, logger)
	if err != nil {
		return
	}

	for f := range monitor.C {
		if local && !f.Host.Equal(localHost) {
			continue
		}

		var data string

		if f.Data != nil {
			// re-encode to remove any whitespace

			var value json.RawMessage

			if err := json.Unmarshal(f.Data, &value); err != nil {
				logger.Print(err)
				continue
			}

			if result, err := json.Marshal(&value); err != nil {
				panic(err)
			} else {
				for _, rune := range string(result) {
					if unicode.IsPrint(rune) && !unicode.IsSpace(rune) {
						data += string(rune)
					} else {
						data += fmt.Sprintf("\\u%04x", rune)
					}
				}
			}
		}

		if _, err = fmt.Printf("%s\t%s\t%s\n", f.Name, f.Host, data); err != nil {
			if pathErr, ok := err.(*os.PathError); ok {
				if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == syscall.EPIPE {
					err = nil
				}
			}
			return
		}
	}

	return
}
