package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	nameq "./go"
)

func name(_, command string) error {
	var (
		nameDir = nameq.DefaultNameDir
		rm      = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] NAME\n\n", command)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
	}

	flag.StringVar(&nameDir, "namedir", nameDir, "dynamic hostname configuration location")
	flag.BoolVar(&rm, "rm", rm, "remove hostname (instead of setting it)")

	flag.Parse()

	var (
		name string
	)

	switch flag.NArg() {
	case 1:
		name = flag.Arg(0)

	default:
		flag.Usage()
		os.Exit(2)
	}

	if rm {
		return nameq.RemoveName(nameDir, name)
	} else {
		return nameq.SetName(nameDir, name)
	}
}

func feature(_, command string) (err error) {
	var (
		featureDir = nameq.DefaultFeatureDir
		rm         = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] NAME [VALUE]\n\n", command)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "VALUE must be valid JSON.  If it's not specified, it will be read from stdin.\n\n")
	}

	flag.StringVar(&featureDir, "featuredir", featureDir, "dynamic feature configuration location")
	flag.BoolVar(&rm, "rm", rm, "remove feature (instead of setting it); VALUE must not be specified")

	flag.Parse()

	var (
		name string
		data []byte
	)

	switch flag.NArg() {
	case 1:
		name = flag.Arg(0)

	case 2:
		name = flag.Arg(0)
		data = []byte(flag.Arg(1))

	default:
		flag.Usage()
		os.Exit(2)
	}

	if rm {
		if data != nil {
			flag.Usage()
			os.Exit(2)
		}

		err = nameq.RemoveFeature(featureDir, name)
	} else {
		if data == nil {
			data, err = ioutil.ReadAll(os.Stdin)
			if err != nil {
				return
			}
		}

		var value json.RawMessage

		if err = json.Unmarshal(data, &value); err != nil {
			return
		}

		if data, err = json.MarshalIndent(&value, "", "\t"); err != nil {
			return
		}

		data = append(data, '\n')

		err = nameq.SetFeature(featureDir, name, data)
	}
	return
}
