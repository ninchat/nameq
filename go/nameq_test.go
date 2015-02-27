package nameq

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"../service"
)

var (
	monitorLogger = log.New(os.Stdout, "", 0)
)

func Test(t *testing.T) {
	dir, err := ioutil.TempDir("", "nameq-go-test-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(dir)

	var (
		nameDir    = filepath.Join(dir, "conf/names")
		featureDir = filepath.Join(dir, "conf/features")
		stateDir   = filepath.Join(dir, "state")
	)

	os.MkdirAll(nameDir, 0700)
	os.MkdirAll(featureDir, 0700)
	os.MkdirAll(stateDir, 0700)

	localAddr := service.GuessAddr()
	if localAddr == "" {
		localAddr = "127.0.0.1"
	}

	go service.Serve(&service.Params{
		Addr:       localAddr,
		NameDir:    nameDir,
		Features:   "{ \"feature-1\": true, \"feature-2\": [1, 2, 3] }",
		FeatureDir: featureDir,
		StateDir:   stateDir,
		SendMode: &service.PacketMode{
			Secret: []byte("swordfish"),
		},
		S3DryRun: true,
	})

	m, err := NewFeatureMonitor(stateDir, monitorLogger)
	if err != nil {
		log.Fatal(err)
	}

	defer m.Close()

	for i := 0; i < 2; i++ {
		f := <-m.C

		var value interface{}

		if err := json.Unmarshal(f.Data, &value); err != nil {
			t.Error(err)
			continue
		}

		t.Logf("feature: name=%s host=%s value=%s", f.Name, f.Host, value)
	}
}
