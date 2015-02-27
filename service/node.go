package service

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

// Node is a JSON-compatible representation of a host.  IPAddr and TimeNs are
// used when sending via UDP, but not when stored in S3.
type Node struct {
	IPAddr   string                      `json:"ip_addr,omitempty"`
	TimeNs   int64                       `json:"time_ns,omitempty"`
	Names    []string                    `json:"names,omitempty"`
	Features map[string]*json.RawMessage `json:"features,omitempty"`
}

type localNode struct {
	ipAddr string
	conn   *net.UDPConn
	mode   *PacketMode
	node   unsafe.Pointer
}

func newLocalNode(ipAddr string, port int, mode *PacketMode) (local *localNode, err error) {
	addr, err := resolveAddr(ipAddr, port)
	if err != nil {
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return
	}

	local = &localNode{
		ipAddr: ipAddr,
		conn:   conn,
		mode:   mode,
	}

	local.setNode(new(Node))

	return
}

func (local *localNode) String() string {
	return local.ipAddr
}

func (local *localNode) getNode() *Node {
	return (*Node)(atomic.LoadPointer(&local.node))
}

func (local *localNode) setNode(node *Node) {
	atomic.StorePointer(&local.node, unsafe.Pointer(node))
}

func (local *localNode) encodeForPacket(w io.Writer) error {
	node := local.getNode()

	return json.NewEncoder(w).Encode(&Node{
		IPAddr:   local.ipAddr,
		TimeNs:   time.Now().UnixNano(),
		Names:    node.Names,
		Features: node.Features,
	})
}

func (local *localNode) marshalForStorage() (data []byte, err error) {
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

func (local *localNode) hasName(name string) (found bool) {
	for _, localName := range local.getNode().Names {
		if name == localName {
			found = true
			break
		}
	}

	return
}

func (local *localNode) updateNames(newNames []string) (update bool) {
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

func (local *localNode) updateFeatures(newFeatures map[string]*json.RawMessage) (update bool) {
	oldNode := local.getNode()

	for name, newValue := range newFeatures {
		oldValue, found := oldNode.Features[name]
		if !found || bytes.Compare(*newValue, *oldValue) != 0 {
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

type remoteNode struct {
	addr *net.UDPAddr
	node *Node
}

func (remote *remoteNode) String() string {
	return remote.node.IPAddr
}

type remoteNodes struct {
	port    int
	lock    sync.RWMutex
	ipAddrs map[string]*remoteNode
	names   map[string][]*remoteNode
}

func newRemoteNodes(port int) *remoteNodes {
	return &remoteNodes{
		port:    port,
		ipAddrs: make(map[string]*remoteNode),
		names:   make(map[string][]*remoteNode),
	}
}

func (remotes *remoteNodes) updatable(ipAddr string, newTime time.Time) bool {
	newTimeNs := newTime.UnixNano()

	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	remote := remotes.ipAddrs[ipAddr]
	return remote == nil || remote.node.TimeNs < newTimeNs
}

func (remotes *remoteNodes) update(newNode *Node, local *localNode, log *Log) (newAddr *net.UDPAddr) {
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

		remote = &remoteNode{
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

func (remotes *remoteNodes) claimName(name string, remote *remoteNode, local *localNode, reclaim bool, log *Log) {
	slice := remotes.names[name]

	if len(slice) == 0 {
		slice = append(slice, remote)
	} else {
		i := sort.Search(len(slice), func(i int) bool {
			return slice[i].node.TimeNs <= remote.node.TimeNs
		})

		slice = append(slice[:i], append([]*remoteNode{remote}, slice[i:]...)...)
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

func (remotes *remoteNodes) disclaimName(name string, remote *remoteNode, local *localNode, reclaim bool, log *Log) {
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

func (remotes *remoteNodes) expire(threshold time.Time, local *localNode, log *Log) {
	thresholdNs := threshold.UnixNano()

	remotes.lock.Lock()
	defer remotes.lock.Unlock()

	var expired []*remoteNode

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

func (remotes *remoteNodes) addrs() (addrs []*net.UDPAddr) {
	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	for _, remote := range remotes.ipAddrs {
		addrs = append(addrs, remote.addr)
	}

	return
}

func (remotes *remoteNodes) nodes() (nodes []*Node) {
	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	for _, remote := range remotes.ipAddrs {
		nodes = append(nodes, remote.node)
	}

	return
}

func (remotes *remoteNodes) resolve(name string) (ip net.IP) {
	remotes.lock.RLock()
	defer remotes.lock.RUnlock()

	slice := remotes.names[name]
	if len(slice) >= 1 {
		ip = slice[0].addr.IP
	}

	return
}

func resolveAddr(ipAddr string, port int) (addr *net.UDPAddr, err error) {
	return net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ipAddr, port))
}
