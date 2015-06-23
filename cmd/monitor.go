package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"
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

func findFeatures(prog, command string) (err error) {
	var (
		stateDir      = nameq.DefaultStateDir
		timeoutString = "infinite"
		single        = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] NAME...\n\n", command)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "This command prints the current hosts which have at least one of the specified features.  If no such hosts exist, it waits until some appear.  If timeout occurs, nothing is printed and the command exits normally.\n\n")
	}

	flag.StringVar(&stateDir, "statedir", stateDir, "runtime state root location")
	flag.StringVar(&timeoutString, "timeout", timeoutString, "wait duration (or 0 to peek quickly)")
	flag.BoolVar(&single, "single", single, "print at most one host")

	flag.Parse()

	timeoutDuration := time.Duration(-1)
	if timeoutString != "infinite" {
		timeoutDuration, err = time.ParseDuration(timeoutString)
		if err != nil {
			return
		}
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	logger := log.New(os.Stderr, prog+": ", 0)

	monitor, err := nameq.NewFeatureMonitor(stateDir, logger)
	if err != nil {
		return
	}

	var timeoutChannel <-chan time.Time
	if timeoutDuration >= 0 {
		timeoutChannel = time.NewTimer(timeoutDuration).C
	}

	accumulator := newStringOrderMap()

	for {
		var f *nameq.Feature

		if monitor.Boot != nil {
			select {
			case f = <-monitor.C:

			case <-monitor.Boot:
				monitor.Boot = nil
				continue
			}
		} else if accumulator.empty() {
			select {
			case f = <-monitor.C:

			case <-timeoutChannel:
				return
			}
		} else {
			select {
			case f = <-monitor.C:

			default:
			}
		}

		if f == nil {
			break
		}

		match := false

		for i := 0; i < flag.NArg(); i++ {
			if flag.Arg(i) == f.Name {
				match = true
				break
			}
		}

		if match {
			host := f.Host.String()

			if f.Data != nil {
				accumulator.add(host)
			} else {
				accumulator.remove(host)
			}
		}
	}

	entries := accumulator.array()

	if single {
		fmt.Println(entries[len(entries)-1])
	} else {
		for _, host := range entries {
			fmt.Println(host)
		}
	}

	return
}

// newStringOrderMap
type stringOrderMap struct {
	order map[string]int
	next  int
}

func newStringOrderMap() *stringOrderMap {
	return &stringOrderMap{
		order: make(map[string]int),
	}
}

func (som *stringOrderMap) empty() bool {
	return len(som.order) == 0
}

func (som *stringOrderMap) add(key string) {
	som.order[key] = som.next
	som.next += 1
}

func (som *stringOrderMap) remove(key string) {
	delete(som.order, key)
}

func (som *stringOrderMap) array() (array []string) {
	for key := range som.order {
		array = append(array, key)
	}

	mosa := &mapOrderStringArray{
		array: array,
		order: som.order,
	}

	sort.Sort(mosa)

	return
}

// mapOrderStringArray
type mapOrderStringArray struct {
	array []string
	order map[string]int
}

func (mosa *mapOrderStringArray) Len() int {
	return len(mosa.array)
}

func (mosa *mapOrderStringArray) Less(i, j int) bool {
	return mosa.order[mosa.array[i]] < mosa.order[mosa.array[j]]
}

func (mosa *mapOrderStringArray) Swap(i, j int) {
	mosa.array[i], mosa.array[j] = mosa.array[j], mosa.array[i]
}

func (mosa *mapOrderStringArray) last() string {
	return mosa.array[len(mosa.array)-1]
}
