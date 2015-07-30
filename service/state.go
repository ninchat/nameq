package service

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	loopbackIPAddr = "127.0.0.1"
)

func initState(local *localNode, remotes *remoteNodes, stateDir string, notifyState <-chan struct{}, log *Log) (err error) {
	featureDir := filepath.Join(stateDir, "features")
	tmpDir := filepath.Join(stateDir, ".tmp")

	if err = os.MkdirAll(featureDir, 0755); err != nil {
		return
	}

	if err = os.MkdirAll(tmpDir, 0700); err != nil {
		return
	}

	go stateLoop(local, remotes, featureDir, tmpDir, notifyState, log)

	return
}

func stateLoop(local *localNode, remotes *remoteNodes, featureDir, tmpDir string, notifyState <-chan struct{}, log *Log) {
	for range notifyState {
		filenames := make(map[string]struct{})

		writeFeatureState(loopbackIPAddr, local.getNode(), featureDir, tmpDir, filenames, log)

		for _, node := range remotes.nodes() {
			writeFeatureState(node.IPAddr, node, featureDir, tmpDir, filenames, log)
		}

		err := filepath.Walk(featureDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Errorf("%s: %s", path, err)
			} else if !info.IsDir() {
				if _, found := filenames[path]; !found {
					log.Debugf("removing file %s", path)
					os.Remove(path)
				}
			}

			return nil
		})
		if err != nil {
			log.Error(err)
		}
	}
}

func writeFeatureState(ipAddr string, node *Node, featureDir, tmpDir string, filenames map[string]struct{}, log *Log) {
	for feature, value := range node.Features {
		dirname := filepath.Join(featureDir, feature)
		filename := filepath.Join(dirname, ipAddr)

		filenames[filename] = struct{}{}

		newData, err := json.MarshalIndent(&value, "", "\t")
		if err != nil {
			panic(err)
		}
		newData = append(newData, byte('\n'))

		if oldData, err := ioutil.ReadFile(filename); err != nil {
			log.Debugf("creating file %s", filename)
		} else if bytes.Compare(oldData, newData) != 0 {
			log.Debugf("updating file %s", filename)
		} else {
			continue
		}

		file, err := ioutil.TempFile(tmpDir, "feature")
		if err != nil {
			log.Error(err)
			continue
		}

		if _, err := file.Write(newData); err != nil {
			file.Close()
			os.Remove(file.Name())
			log.Error(err)
			continue
		}

		if err := file.Chmod(0444); err != nil {
			os.Remove(file.Name())
			log.Error(err)
			continue
		}

		if err := file.Close(); err != nil {
			os.Remove(file.Name())
			log.Error(err)
			continue
		}

		if err := os.MkdirAll(dirname, 0755); err != nil {
			os.Remove(file.Name())
			log.Error(err)
			continue
		}

		if err := os.Rename(file.Name(), filename); err != nil {
			os.Remove(file.Name())
			log.Error(err)
			continue
		}
	}
}
