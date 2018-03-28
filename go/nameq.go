// Package nameq is an application support library for using the information
// exported by the nameq service.
package nameq

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

const (
	// The directory used when an empty string is given to SetFeature or RemoveFeature.
	DefaultFeatureDir = "/etc/nameq/features"

	// The directory used when an empty string is given to NewFeatureMonitor.
	DefaultStateDir = "/run/nameq/state"
)

// SetFeature adds or updates a local feature.  data must be a valid JSON
// document.  DefaultFeatureDir is used if an empty string is given.  Redundant
// calls are ok.
func SetFeature(featureDir, name string, data []byte) error {
	if featureDir == "" {
		featureDir = DefaultFeatureDir
	}

	if len(data) == 0 {
		panic("no data for feature")
	}

	return createConfigFile(featureDir, name, data)
}

// RemoveFeature removes a local feature.  DefaultFeatureDir is used if an
// empty string is given.  Redundant calls are ok.
func RemoveFeature(featureDir, name string) error {
	if featureDir == "" {
		featureDir = DefaultFeatureDir
	}

	return removeConfigFile(featureDir, name)
}

func createConfigFile(dir, name string, data []byte) (err error) {
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}

	tmpDir := filepath.Join(dir, ".tmp")
	os.Mkdir(tmpDir, 0700)

	tmpPath := filepath.Join(tmpDir, name)

	if err = ioutil.WriteFile(tmpPath, data, 0644); err != nil {
		return
	}

	path := filepath.Join(dir, name)

	if err = os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
	}
	return
}

func removeConfigFile(dir, name string) (err error) {
	if err = os.Remove(filepath.Join(dir, name)); err != nil {
		if err.(*os.PathError).Err == syscall.ENOENT {
			err = nil
		}
	}
	return
}

// Feature represents a momentary state of a feature on a host.
type Feature struct {
	Name string // Name of the feature.
	Host net.IP // IPv4 or IPv6 address of the host where the feature exists.
	Data []byte // JSON value if feature added or updated, or nil if removed.
}

// String returns the feature name.
func (f *Feature) String() string {
	return f.Name
}

// Logger is a subset of the standard log.Logger.
type Logger interface {
	Print(v ...interface{})
}

// FeatureMonitor watches the nameq runtime state for changes.
type FeatureMonitor struct {
	// Produces current information, followed by updates in real time.  There
	// may be seemingly redundant entries when a feature is updated rapidly.
	C <-chan *Feature

	// Boot is closed after all existing features have been delivered via C.
	// Client code is free to set this member to nil after it has been closed.
	Boot <-chan struct{}

	logger  Logger
	closed  chan struct{}
	watcher *fsnotify.Watcher
	queued  []*Feature
}

// NewFeatureMonitor watches the specified state directory, or the default
// state directory if an empty string is given.  The directory must exist.  The
// logger is used for I/O errors, unless nil.
func NewFeatureMonitor(stateDir string, logger Logger) (m *FeatureMonitor, err error) {
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

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}

	if err = watcher.Add(featureDir); err != nil {
		watcher.Close()
		return
	}

	c := make(chan *Feature)
	boot := make(chan struct{})

	m = &FeatureMonitor{
		C:       c,
		Boot:    boot,
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

	m.queued = append(m.queued, nil) // indicates end of boot

	go m.watchLoop(c, boot, featureDir)

	return
}

// Close stops watching immediately and closes the channel.
func (m *FeatureMonitor) Close() {
	select {
	case m.closed <- struct{}{}:
	default:
	}
}

func (m *FeatureMonitor) watchLoop(c chan<- *Feature, boot chan<- struct{}, featureDir string) {
	defer m.watcher.Close()
	defer close(c)

	// Stripping this prefix from watched names converts them into relative paths
	namePrefixLen := len(featureDir) + 1 // separator

	for {
		input := m.watcher.Events
		output := c
		var outstanding *Feature

		if len(m.queued) > 0 {
			outstanding = m.queued[0]
			input = nil

			if outstanding == nil { // end of boot indicator
				close(boot)
				m.queued = m.queued[1:]
				continue
			}
		} else {
			output = nil
		}

		select {
		case e := <-input:
			if e.Name == featureDir {
				// First level
				if e.Op&fsnotify.Remove != 0 {
					return
				}
			} else {
				relName := e.Name[namePrefixLen:]
				if !strings.ContainsRune(relName, filepath.Separator) {
					// Second level
					m.addFeature(e.Name)
				} else {
					// Deeper level
					hostname := filepath.Base(e.Name)
					if e.Op&fsnotify.Create != 0 {
						m.addHost(hostname, e.Name)
					}
					if e.Op&fsnotify.Remove != 0 {
						m.removeHost(hostname, e.Name)
					}
				}
			}

		case err := <-m.watcher.Errors:
			m.log(err)

		case output <- outstanding:
			m.queued = m.queued[1:]

		case <-m.closed:
			return
		}
	}
}

func (m *FeatureMonitor) addFeature(dir string) {
	if err := m.watcher.Add(dir); err != nil {
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
