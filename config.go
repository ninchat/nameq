package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"./service"
)

func configureName() {
	var (
		nameDir = service.DefaultNameDir
		rm      = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] NAME\n\n", subprog)
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
		removeFile(nameDir, name)
	} else {
		createFile(nameDir, name, nil)
	}
}

func configureFeature() {
	var (
		featureDir = service.DefaultFeatureDir
		rm         = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] NAME [VALUE]\n\n", subprog)
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

		removeFile(featureDir, name)
	} else {
		if data == nil {
			var err error

			data, err = ioutil.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s\n", subprog, err)
				os.Exit(1)
			}
		}

		var value json.RawMessage

		if err := json.Unmarshal(data, &value); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", subprog, err)
			os.Exit(2)
		}

		data, err := json.MarshalIndent(&value, "", "\t")
		if err != nil {
			panic(err)
		}

		data = append(data, '\n')

		createFile(featureDir, name, data)
	}
}

func createFile(dir, name string, data []byte) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", subprog, err)
		os.Exit(1)
	}

	tmpDir := filepath.Join(dir, ".tmp")
	os.Mkdir(tmpDir, 0700)

	tmpPath := filepath.Join(tmpDir, name)

	if err := ioutil.WriteFile(tmpPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", subprog, err)
		os.Exit(1)
	}

	path := filepath.Join(dir, name)

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "%s: %s\n", subprog, err)
		os.Exit(1)
	}
}

func removeFile(dir, name string) {
	path := filepath.Join(dir, name)

	if err := os.Remove(path); err != nil {
		pathErr := err.(*os.PathError)
		if pathErr.Err != syscall.ENOENT {
			fmt.Fprintf(os.Stderr, "%s: %s\n", subprog, err)
			os.Exit(1)
		}
	}
}
