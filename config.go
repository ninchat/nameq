package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/exp/inotify"
)

const (
	watchEvents = inotify.IN_ATTRIB | inotify.IN_CLOSE_WRITE | inotify.IN_CREATE | inotify.IN_DELETE | inotify.IN_MOVED_FROM | inotify.IN_MOVED_TO
)

var (
	nameRE    = regexp.MustCompile("^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$")
	featureRE = regexp.MustCompile("^[a-zA-Z0-9-_]+$")
)

func watchConfig(dir string, re *regexp.Regexp, log *Log, handler func(filenames []string)) (err error) {
	if dir == "" {
		return
	}

	w, err := inotify.NewWatcher()
	if err != nil {
		return
	}

	if err = w.AddWatch(dir, inotify.IN_ONLYDIR|watchEvents); err != nil {
		w.Close()
		return
	}

	scanConfig(dir, re, handler, log)

	go scanConfigLoop(dir, re, handler, w, log)

	return
}

func scanConfigLoop(dir string, re *regexp.Regexp, handler func(filenames []string), w *inotify.Watcher, log *Log) {
	for {
		select {
		case <-w.Event:
			scanConfig(dir, re, handler, log)

		case err := <-w.Error:
			log.Error(err)
		}
	}
}

func scanConfig(dir string, re *regexp.Regexp, handler func(filenames []string), log *Log) {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Error(err)
		return
	}

	var filenames []string

	for _, info := range infos {
		if !info.IsDir() && re.MatchString(info.Name()) {
			filenames = append(filenames, info.Name())
		}
	}

	handler(filenames)
}

func initNameConfig(local *LocalNode, arg, dir string, notify chan<- struct{}, log *Log) error {
	argNames := strings.Fields(arg)

	return watchConfig(dir, nameRE, log, func(filenames []string) {
		var names []string
		copy(names, argNames)

		for _, filename := range filenames {
			name := strings.ToLower(filename)
			found := false

			for _, x := range names {
				if x == name {
					found = true
					break
				}
			}

			if !found {
				names = append(names, name)
			}
		}

		if local.updateNames(names) {
			select {
			case notify <- struct{}{}:
			default:
			}

			log.Infof("local names: %s", strings.Join(names, " "))
		}
	})
}

func initFeatureConfig(local *LocalNode, arg, dir string, notify chan<- struct{}, log *Log) (err error) {
	var argFeatures map[string]json.RawMessage

	if arg != "" {
		if err = json.Unmarshal([]byte(arg), &argFeatures); err != nil {
			return
		}
	} else {
		argFeatures = make(map[string]json.RawMessage)
	}

	return watchConfig(dir, featureRE, log, func(filenames []string) {
		features := make(map[string]json.RawMessage)

		for name, value := range argFeatures {
			features[name] = value
		}

		for _, name := range filenames {
			path := filepath.Join(dir, name)

			data, err := ioutil.ReadFile(path)
			if err != nil {
				log.Error(err)
				continue
			}

			if len(bytes.TrimSpace(data)) == 0 {
				delete(features, name)
				continue
			}

			var value json.RawMessage

			if err = json.Unmarshal(data, &value); err != nil {
				log.Errorf("%s: %s", path, err)
				continue
			}

			features[name] = value
		}

		if local.updateFeatures(features) {
			select {
			case notify <- struct{}{}:
			default:
			}

			var names []string

			for name := range features {
				names = append(names, name)
			}

			sort.Strings(names)

			log.Infof("local features: %s", strings.Join(names, " "))
		}
	})
}
