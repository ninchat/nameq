package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
		Features: node.Features,
	})
}

func (local *localNode) marshalForStorage() (data []byte, err error) {
	node := local.getNode()

	data, err = json.MarshalIndent(&Node{
		Features: node.Features,
	}, "", "\t")

	if err == nil {
		data = append(data, byte('\n'))
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
			Features: newFeatures,
		})
	}

	return
}

func (local *localNode) empty() (empty *localNode) {
	empty = &localNode{
		ipAddr: local.ipAddr,
		conn:   local.conn,
		mode:   local.mode,
	}
	empty.setNode(new(Node))
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
}

func newRemoteNodes(port int) *remoteNodes {
	return &remoteNodes{
		port:    port,
		ipAddrs: make(map[string]*remoteNode),
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

	if remote := remotes.ipAddrs[newNode.IPAddr]; remote != nil {
		if newNode.TimeNs > remote.node.TimeNs {
			remote.node = newNode
		}
	} else {
		newAddr, _ = resolveAddr(newNode.IPAddr, remotes.port)

		remotes.ipAddrs[newNode.IPAddr] = &remoteNode{
			addr: newAddr,
			node: newNode,
		}
	}

	return
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

func resolveAddr(ipAddr string, port int) (addr *net.UDPAddr, err error) {
	return net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ipAddr, port))
}
