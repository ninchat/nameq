// Application support library for using the information exported by nameq.
package nameq

import (
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/exp/inotify"
)

const (
	// The directory used when an empty string is given to NewFeatureMonitor.
	DefaultStateDir = "/run/nameq/state"
)

// Feature represents a momentary state of a feature on a host.
type Feature struct {
	Name string // Name of the feature.
	Host net.IP // IPv4 or IPv6 address of the host where the feature exists.
	Data []byte // JSON value if feature added or updated, or nil if removed.
}

// FeatureMonitor watches the nameq runtime state for changes.
type FeatureMonitor struct {
	// Produces current information, followed by updates in real time.  There
	// may be seemingly redundant entries when a feature is updated rapidly.
	C <-chan *Feature

	logger  *log.Logger
	closed  chan struct{}
	watcher *inotify.Watcher
	queued  []*Feature
}

// NewFeatureMonitor watches the specified state directory, or the default
// state directory if an empty string is given.  The directory must exist.  The
// logger is used for I/O errors, unless nil.
func NewFeatureMonitor(stateDir string, logger *log.Logger) (m *FeatureMonitor, err error) {
	if stateDir == "" {
		stateDir = DefaultStateDir
	}

	featureDir := filepath.Join(stateDir, "features")

	if err = os.Mkdir(featureDir, 0755); err != nil {
		if info, statErr := os.Stat(featureDir); statErr != nil || !info.IsDir() {
			return
		}
		err = nil
	}

	if featureDir, err = filepath.Abs(featureDir); err != nil {
		return
	}

	if featureDir, err = filepath.EvalSymlinks(featureDir); err != nil {
		return
	}

	watcher, err := inotify.NewWatcher()
	if err != nil {
		return
	}

	if err = watcher.AddWatch(featureDir, inotify.IN_ONLYDIR|inotify.IN_CREATE|inotify.IN_DELETE|inotify.IN_DELETE_SELF); err != nil {
		watcher.Close()
		return
	}

	c := make(chan *Feature)

	m = &FeatureMonitor{
		C:       c,
		logger:  logger,
		closed:  make(chan struct{}, 1),
		watcher: watcher,
	}

	if infos, err := ioutil.ReadDir(featureDir); err == nil {
		for _, info := range infos {
			m.addFeature(filepath.Join(featureDir, info.Name()))
		}
	} else {
		m.log(err)
	}

	go m.watchLoop(c, featureDir)

	return
}

// Close stops watching immediately and closes the channel.
func (m *FeatureMonitor) Close() {
	select {
	case m.closed <- struct{}{}:
	default:
	}
}

func (m *FeatureMonitor) watchLoop(c chan<- *Feature, featureDir string) {
	defer m.watcher.Close()
	defer close(c)

	for {
		input := m.watcher.Event
		output := c
		var outstanding *Feature

		if len(m.queued) > 0 {
			outstanding = m.queued[0]
			input = nil
		} else {
			output = nil
		}

		select {
		case e := <-input:
			if (e.Mask & inotify.IN_CREATE) != 0 {
				m.addFeature(e.Name)
			}

			if (e.Mask&inotify.IN_DELETE) != 0 && filepath.Dir(e.Name) != featureDir {
				m.removeHost(filepath.Base(e.Name), e.Name)
			}

			if (e.Mask & inotify.IN_DELETE_SELF) != 0 {
				return
			}

			if (e.Mask & inotify.IN_MOVED_TO) != 0 {
				m.addHost(filepath.Base(e.Name), e.Name)
			}

		case err := <-m.watcher.Error:
			m.log(err)

		case output <- outstanding:
			m.queued = m.queued[1:]

		case <-m.closed:
			return
		}
	}
}

func (m *FeatureMonitor) addFeature(dir string) {
	if err := m.watcher.AddWatch(dir, inotify.IN_ONLYDIR|inotify.IN_DELETE|inotify.IN_MOVED_TO); err != nil {
		m.log(err)
		return
	}

	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		m.log(err)
		return
	}

	for _, info := range infos {
		m.addHost(info.Name(), filepath.Join(dir, info.Name()))
	}
}

func (m *FeatureMonitor) addHost(hostname, path string) {
	host := m.parseHost(hostname, path)
	if host == nil {
		return
	}

	// File may not exist if we were too slow; just skip this one silently, the
	// delete event will follow.  It will appear spurious for the client code,
	// but that's okay.

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		m.log(err)
		return
	}

	m.queued = append(m.queued, &Feature{
		Name: filepath.Base(filepath.Dir(path)),
		Host: host,
		Data: data,
	})
}

func (m *FeatureMonitor) removeHost(hostname, path string) {
	host := m.parseHost(hostname, path)
	if host == nil {
		return
	}

	// File may exist if we were too slow... see the comment in addHost.

	if _, err := os.Stat(path); err == nil {
		return
	}

	m.queued = append(m.queued, &Feature{
		Name: filepath.Base(filepath.Dir(path)),
		Host: host,
	})
}

func (m *FeatureMonitor) parseHost(name string, path string) (host net.IP) {
	if host = net.ParseIP(name); host == nil {
		m.log("unable to parse filename: ", path)
	}

	return
}

func (m *FeatureMonitor) log(args ...interface{}) {
	if m.logger != nil {
		m.logger.Print(args...)
	}
}
