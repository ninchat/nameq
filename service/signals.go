package service

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/context"
)

// HandleSignals cancels on termination or interrupt.
func HandleSignals(cancel context.CancelFunc) {
	c := make(chan os.Signal)
	signal.Notify(c)

	go func() {
		defer cancel()

		for s := range c {
			switch s {
			case syscall.SIGTERM, syscall.SIGINT:
				return
			}
		}
	}()
}
