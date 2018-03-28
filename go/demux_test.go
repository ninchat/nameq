package nameq_test

import (
	"testing"

	nameq "github.com/ninchat/nameq/go"
)

func TestDemux(t *testing.T) {
	input := make(chan *nameq.Feature)
	foo := make(chan *nameq.Feature)
	bar1 := make(chan *nameq.Feature)
	bar2 := make(chan *nameq.Feature, 10)
	baz := make(chan *nameq.Feature, 1)

	demux := nameq.FeatureDemux{
		"foo": {
			nameq.FeatureBuffer(foo),
		},
		"bar": {
			nameq.FeatureBuffer(bar1),
			bar2,
		},
	}

	demux.Add("baz", baz)
	demux.Start(input)

	done := make(chan struct{})

	go consume("foo", foo, done)
	go consume("bar", bar1, done)
	go consume("bar", bar2, done)
	go consume("baz", baz, done)

	for i := 0; i < 1234; i++ {
		input <- &nameq.Feature{Name: "foo"}
		input <- &nameq.Feature{Name: "bar"}
		input <- &nameq.Feature{Name: "baz"}
		input <- &nameq.Feature{Name: "quux"}
	}

	close(input)

	for i := 0; i < 3; i++ {
		<-done
	}
}

func consume(s string, c <-chan *nameq.Feature, done chan<- struct{}) {
	defer func() {
		done <- struct{}{}
	}()

	for i := range c {
		if i.Name != s {
			panic(i)
		}
	}
}
