package nameq

// FeatureDemux can be used to forward Features from FeatureMonitor to multiple
// subscribers based on the Name field.
//
// Example:
//
//    monitor, err := nameq.NewFeatureMonitor("", nil)
//    if err != nil {
//        return
//    }
//    defer monitor.Close()
//
//    foo := make(chan *nameq.Feature)
//    bar1 := make(chan *nameq.Feature)
//    bar2 := make(chan *nameq.Feature)
//
//    nameq.FeatureDemux{
//        "foo": {
//            nameq.FeatureBuffer(foo),
//        },
//        "bar": {
//            nameq.FeatureBuffer(bar1),
//            nameq.FeatureBuffer(bar2),
//        },
//    }.Start(monitor.C)
//
//    go consume(foo)
//    go consume(bar1)
//    go consume(bar2)
//
type FeatureDemux map[string][]chan<- *Feature

// Add adds a subscriber.  The supplied channel should be buffered.
func (queues FeatureDemux) Add(name string, output chan<- *Feature) {
	queues[name] = append(queues[name], output)
}

// AddBuffered adds a subscriber with unbounded buffering.  The supplied
// channel doesn't need to be buffered.
func (queues FeatureDemux) AddBuffer(name string, output chan<- *Feature) {
	queues[name] = append(queues[name], FeatureBuffer(output))
}

// Start after adding all subscribers.  Supply the FeatureMonitor's channel as
// the argument.
func (outputs FeatureDemux) Start(input <-chan *Feature) {
	go func() {
		defer func() {
			for _, array := range outputs {
				for _, output := range array {
					close(output)
				}
			}
		}()

		for f := range input {
			for _, output := range outputs[f.Name] {
				output <- f
			}
		}
	}()
}

// FeatureBuffer is an unbounded buffering adapter for Feature channels.  The
// supplied channel doesn't need to be buffered.
func FeatureBuffer(output chan<- *Feature) chan<- *Feature {
	queue := make(chan *Feature)

	go func() {
		defer close(output)

		var buffer []*Feature

		for {
			var (
				next *Feature
				out  chan<- *Feature
			)

			if len(buffer) > 0 {
				next = buffer[0]
				out = output
			} else if queue == nil {
				return
			}

			select {
			case f, ok := <-queue:
				if ok {
					buffer = append(buffer, f)
				} else {
					queue = nil
				}

			case out <- next:
				buffer = buffer[1:]
			}
		}
	}()

	return queue
}
