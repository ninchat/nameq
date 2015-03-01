package service

import (
	"net"
)

// Default values for some Params.
const (
	DefaultPort       = 17106
	DefaultNameDir    = "/etc/nameq/names"
	DefaultFeatureDir = "/etc/nameq/features"
	DefaultStateDir   = "/run/nameq/state"
	DefaultResolvConf = "/etc/nameq/resolv.conf"
)

// Params of the service.
type Params struct {
	Addr         string // Required.
	Port         int
	Names        string
	NameDir      string
	Features     string
	FeatureDir   string
	StateDir     string
	DNSAddr      string // Required if DNSTCP or DNSUDP is set.
	DNSTCP       bool
	DNSUDP       bool
	ResolvConf   string
	SendMode     *PacketMode         // Required.
	ReceiveModes map[int]*PacketMode // Defaults to SendMode.
	S3Creds      []byte
	S3Region     string // Required unless S3DryRun is set.
	S3Bucket     string // Required unless S3DryRun is set.
	S3Prefix     string
	S3DryRun     bool
	Log          Log
}

// GuessAddr tries to find a local interface suitable for the Addr parameter.
func GuessAddr() string {
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if netAddr, ok := addr.(*net.IPNet); ok && netAddr.IP.IsGlobalUnicast() {
				return netAddr.IP.String()
			}
		}
	}

	return ""
}

// Serve indefinitely.
func Serve(p *Params) (err error) {
	if p.Port == 0 {
		p.Port = DefaultPort
	}
	if p.NameDir == "" {
		p.NameDir = DefaultNameDir
	}
	if p.FeatureDir == "" {
		p.FeatureDir = DefaultFeatureDir
	}
	if p.StateDir == "" {
		p.StateDir = DefaultStateDir
	}
	if p.ResolvConf == "" {
		p.ResolvConf = DefaultResolvConf
	}
	if p.ReceiveModes == nil {
		p.ReceiveModes = map[int]*PacketMode{
			p.SendMode.Id: p.SendMode,
		}
	}

	log := &p.Log

	local, err := newLocalNode(p.Addr, p.Port, p.SendMode)
	if err != nil {
		return
	}

	remotes := newRemoteNodes(p.Port)

	var (
		notify         = make(chan struct{}, 1)
		notifyState    = make(chan struct{}, 1)
		notifyStorage  = make(chan struct{}, 1)
		notifyTransmit = make(chan struct{}, 1)
		reply          = make(chan []*net.UDPAddr, 10)
	)

	if err = initNameConfig(local, p.Names, p.NameDir, notify, log); err != nil {
		return
	}

	if err = initFeatureConfig(local, p.Features, p.FeatureDir, notify, log); err != nil {
		return
	}

	if err = initState(local, remotes, p.StateDir, notifyState, log); err != nil {
		return
	}

	if err = initDNS(local, remotes, p.DNSAddr, p.DNSTCP, p.DNSUDP, p.ResolvConf, log); err != nil {
		return
	}

	go receiveLoop(local, remotes, p.ReceiveModes, notifyState, reply, log)
	go transmitLoop(local, remotes, notifyTransmit, reply, log)

	if err = initStorage(local, remotes, notifyStorage, reply, p.S3Creds, p.S3Region, p.S3Bucket, p.S3Prefix, p.S3DryRun, log); err != nil {
		return
	}

	var (
		forwardState    chan<- struct{}
		forwardStorage  chan<- struct{}
		forwardTransmit chan<- struct{}
	)

	for {
		select {
		case <-notify:
			forwardState = notifyState
			forwardStorage = notifyStorage
			forwardTransmit = notifyTransmit

		case forwardState <- struct{}{}:
			forwardState = nil

		case forwardStorage <- struct{}{}:
			forwardStorage = nil

		case forwardTransmit <- struct{}{}:
			forwardTransmit = nil
		}
	}
}
