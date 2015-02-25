package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

func resolveAddr(ipAddr string, port int) (addr *net.UDPAddr, err error) {
	return net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ipAddr, port))
}

type Node struct {
	IPAddr   string                     `json:"ip_addr,omitempty"`
	TimeNs   int64                      `json:"time_ns,omitempty"`
	Names    []string                   `json:"names,omitempty"`
	Features map[string]json.RawMessage `json:"features,omitempty"`
}

type LocalNode struct {
	ipAddr string
	conn   *net.UDPConn
	mode   *Mode
	node   unsafe.Pointer
}

func newLocalNode(ipAddr string, port int, mode *Mode) (local *LocalNode, err error) {
	addr, err := resolveAddr(ipAddr, port)
	if err != nil {
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return
	}

	local = &LocalNode{
		ipAddr: ipAddr,
		conn:   conn,
		mode:   mode,
	}

	local.setNode(new(Node))

	return
}

func (local *LocalNode) String() string {
	return local.ipAddr
}

func (local *LocalNode) getNode() *Node {
	return (*Node)(atomic.LoadPointer(&local.node))
}

func (local *LocalNode) setNode(node *Node) {
	atomic.StorePointer(&local.node, unsafe.Pointer(node))
}

func (local *LocalNode) encodeForPacket(w io.Writer) error {
	node := local.getNode()

	return json.NewEncoder(w).Encode(&Node{
		IPAddr:   local.ipAddr,
		TimeNs:   time.Now().UnixNano(),
		Names:    node.Names,
		Features: node.Features,
	})
}

func (local *LocalNode) marshalForStorage() (data []byte, err error) {
	node := local.getNode()

	data, err = json.MarshalIndent(&Node{
		Names:    node.Names,
		Features: node.Features,
	}, "", "\t")

	if err == nil {
		data = append(data, byte('\n'))
	}

	return
}

func (local *LocalNode) hasName(name string) (found bool) {
	for _, localName := range local.getNode().Names {
		if name == localName {
			found = true
			break
		}
	}

	return
}

func (local *LocalNode) updateNames(newNames []string) (update bool) {
	oldNode := local.getNode()

	sort.Strings(newNames)

	if len(oldNode.Names) != len(newNames) {
		update = true
	} else {
		for i := 0; i < len(newNames); i++ {
			if oldNode.Names[i] != newNames[i] {
				update = true
				break
			}
		}
	}

	if update {
		local.setNode(&Node{
			Names:    newNames,
			Features: oldNode.Features,
		})
	}

	return
}

func (local *LocalNode) updateFeatures(newFeatures map[string]json.RawMessage) (update bool) {
	oldNode := local.getNode()

	for name, newValue := range newFeatures {
		oldValue, found := oldNode.Features[name]
		if !found || bytes.Compare(newValue, oldValue) != 0 {
			update = true
			break
		}
	}

	if !update {
		for name := range oldNode.Features {
			_, found := newFeatures[name]
			if !found {
				update = true
				break
			}
		}
	}

	if update {
		local.setNode(&Node{
			Names:    oldNode.Names,
			Features: newFeatures,
		})
	}

	return
}

type RemoteNode struct {
	addr *net.UDPAddr
	node *Node
}

func (remote *RemoteNode) String() string {
	return remote.node.IPAddr
}

type RemoteNodes struct {
	port    int
	lock    sync.RWMutex
	ipAddrs map[string]*RemoteNode
	names   map[string][]*RemoteNode
}

func newRemoteNodes(port int) *RemoteNodes {
	return &RemoteNodes{
		port:    port,
		ipAddrs: make(map[string]*RemoteNode),
		names:   make(map[string][]*RemoteNode),
	}
}

func (remotes *RemoteNodes) updatable(ipAddr string, newTime time.Time) bool {
	newTimeNs := newTime.UnixNano()

	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	remote := remotes.ipAddrs[ipAddr]
	return remote == nil || remote.node.TimeNs < newTimeNs
}

func (remotes *RemoteNodes) update(newNode *Node, local *LocalNode, log *Log) (newAddr *net.UDPAddr) {
	remotes.lock.Lock()
	defer remotes.lock.Unlock()

	var oldNames []string

	remote := remotes.ipAddrs[newNode.IPAddr]
	if remote != nil {
		if remote.node.TimeNs > newNode.TimeNs {
			return
		}

		oldNames = remote.node.Names
	}

	sort.Strings(newNode.Names)

	oldHas := make(map[string]struct{})
	newHas := make(map[string]struct{})

	for _, name := range oldNames {
		oldHas[name] = struct{}{}
	}

	for _, name := range newNode.Names {
		newHas[name] = struct{}{}
	}

	for _, name := range oldNames {
		_, reclaim := newHas[name]
		remotes.disclaimName(name, remote, local, reclaim, log)
	}

	if remote == nil {
		newAddr, _ = resolveAddr(newNode.IPAddr, remotes.port)

		remote = &RemoteNode{
			addr: newAddr,
		}

		remotes.ipAddrs[newNode.IPAddr] = remote
	}

	remote.node = newNode

	for _, name := range newNode.Names {
		_, reclaim := oldHas[name]
		remotes.claimName(name, remote, local, reclaim, log)
	}

	return
}

func (remotes *RemoteNodes) claimName(name string, remote *RemoteNode, local *LocalNode, reclaim bool, log *Log) {
	slice := remotes.names[name]

	if len(slice) == 0 {
		slice = append(slice, remote)
	} else {
		i := sort.Search(len(slice), func(i int) bool {
			return slice[i].node.TimeNs <= remote.node.TimeNs
		})

		slice = append(slice[:i], append([]*RemoteNode{remote}, slice[i:]...)...)
	}

	remotes.names[name] = slice

	if !reclaim {
		localHas := local.hasName(name)
		if localHas || len(slice) > 1 {
			var s string
			for _, r := range slice {
				s += " " + r.String()
			}
			if localHas {
				log.Infof("other claims for local name %s:%s", name, s)
			} else {
				log.Infof("multiple claims for name %s:%s", name, s)
			}
		}
	}
}

func (remotes *RemoteNodes) disclaimName(name string, remote *RemoteNode, local *LocalNode, reclaim bool, log *Log) {
	slice := remotes.names[name]

	if len(slice) == 1 {
		if reclaim {
			remotes.names[name] = slice[:0]
		} else {
			delete(remotes.names, name)
		}
	} else {
		i := sort.Search(len(slice), func(i int) bool {
			return slice[i].node.TimeNs <= remote.node.TimeNs
		})

		for slice[i] != remote {
			i++
		}

		remotes.names[name] = append(slice[:i], slice[i+1:]...)
	}

	if !reclaim && !(local.hasName(name) && len(slice) > 0) {
		log.Infof("claim for name %s was settled", name)
	}
}

func (remotes *RemoteNodes) expire(threshold time.Time, local *LocalNode, log *Log) {
	thresholdNs := threshold.UnixNano()

	remotes.lock.Lock()
	defer remotes.lock.Unlock()

	var expired []*RemoteNode

	for _, remote := range remotes.ipAddrs {
		if remote.node.TimeNs < thresholdNs {
			log.Infof("expiring %s", remote)
			expired = append(expired, remote)
		}
	}

	for _, remote := range expired {
		for _, name := range remote.node.Names {
			remotes.disclaimName(name, remote, local, false, log)
		}

		delete(remotes.ipAddrs, remote.node.IPAddr)
	}
}

func (remotes *RemoteNodes) addrs() (addrs []*net.UDPAddr) {
	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	for _, remote := range remotes.ipAddrs {
		addrs = append(addrs, remote.addr)
	}

	return
}

func (remotes *RemoteNodes) nodes() (nodes []*Node) {
	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	for _, remote := range remotes.ipAddrs {
		nodes = append(nodes, remote.node)
	}

	return
}

func (remotes *RemoteNodes) resolve(name string) (ip net.IP) {
	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	slice := remotes.names[name]
	if len(slice) >= 1 {
		ip = slice[0].addr.IP
	}

	return
}
