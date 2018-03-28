package nameq_test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	nameq "github.com/ninchat/nameq/go"
	"github.com/ninchat/nameq/service"
)

var (
	serviceErrorLogger = log.New(os.Stderr, "service error: ", 0)
	serviceInfoLogger  = log.New(os.Stderr, "service info: ", 0)
	serviceDebugLogger = log.New(os.Stderr, "service debug: ", 0)
	monitorLogger      = log.New(os.Stderr, "monitor: ", 0)
)

func Test(t *testing.T) {
	dir, err := ioutil.TempDir("", "nameq-go-test-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(dir)

	var (
		featureDir = filepath.Join(dir, "conf/features")
		stateDir   = filepath.Join(dir, "state")
	)

	os.MkdirAll(featureDir, 0700)
	os.MkdirAll(stateDir, 0700)

	localAddr := service.GuessAddr()
	if localAddr == "" {
		localAddr = "127.0.0.1"
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)

		service.Serve(ctx, &service.Params{
			Addr:       localAddr,
			Features:   "{ \"feature-1\": true, \"feature-2\": [1, 2, 3] }",
			FeatureDir: featureDir,
			StateDir:   stateDir,
			SendMode: &service.PacketMode{
				Secret: []byte("swordfish"),
			},
			S3DryRun: true,
			Log: service.Log{
				ErrorLogger: serviceErrorLogger,
				InfoLogger:  serviceInfoLogger,
				DebugLogger: serviceDebugLogger,
			},
		})
	}()

	m, err := nameq.NewFeatureMonitor(stateDir, monitorLogger)
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

	cancel()
	<-done
}
