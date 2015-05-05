package service

import (
	"net"
	"time"

	"golang.org/x/net/context"
)

const (
	safeDatagramSize = 512
	maxDatagramSize  = 65535

	minTransmitInterval = time.Second * 20
	maxTransmitInterval = time.Second * 40

	latencyTolerance = time.Second * 15
)

func randomTransmitInterval() time.Duration {
	return randomDuration(minTransmitInterval, maxTransmitInterval)
}

func transmitLoop(ctx context.Context, local *localNode, remotes *remoteNodes, notify <-chan struct{}, reply <-chan []*net.UDPAddr, done chan<- struct{}, log *Log) {
	defer func() {
		transmit(local.empty(), remotes.addrs(), log)
		close(done)
	}()

	var replyTo []*net.UDPAddr

	timer := time.NewTimer(randomTransmitInterval())

	for {
		addrs := replyTo
		replyTo = nil

		if addrs == nil {
			addrs = remotes.addrs()
		}

		transmit(local, addrs, log)

		select {
		case addrs := <-reply:
			for _, addr := range addrs {
				found := false

				for _, x := range replyTo {
					if x == addr {
						found = true
						break
					}
				}

				if !found {
					replyTo = append(replyTo, addr)
				}
			}

		case <-notify:
			timer.Reset(randomTransmitInterval())

		case <-timer.C:
			timer.Reset(randomTransmitInterval())

		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

func transmit(local *localNode, addrs []*net.UDPAddr, log *Log) {
	data, err := marshalPacket(local)
	if err != nil {
		panic(err)
	}

	if len(data) > safeDatagramSize {
		log.Errorf("sending dangerously large packet: %d bytes", len(data))
	}

	for _, i := range random.Perm(len(addrs)) {
		log.Debugf("sending to %s", addrs[i].IP)

		if _, err := local.conn.WriteToUDP(data, addrs[i]); err != nil {
			log.Error(err)
		}
	}
}

func receiveLoop(local *localNode, remotes *remoteNodes, modes map[int]*PacketMode, notify chan<- struct{}, reply chan<- []*net.UDPAddr, log *Log) {
	buf := make([]byte, maxDatagramSize)

	for {
		n, originAddr, err := local.conn.ReadFromUDP(buf)
		if err != nil {
			log.Error(err)
			continue
		}

		log.Debugf("received from %s", originAddr.IP)

		if !originAddr.IP.IsGlobalUnicast() {
			log.Errorf("bad origin address: %s", originAddr.IP)
			continue
		}

		data := buf[:n]

		node, err := unmarshalPacket(data, modes)
		if err != nil {
			log.Error(err)
			continue
		}

		if err := verifyPacketOrigin(node, originAddr); err != nil {
			log.Error(err)
			continue
		}

		latency := time.Now().Sub(time.Unix(0, node.TimeNs))
		if latency > latencyTolerance {
			log.Errorf("intolerable %s latency %s", originAddr.IP, latency)
			continue
		}

		newAddr := remotes.update(node, local, log)

		select {
		case notify <- struct{}{}:
		default:
		}

		if newAddr != nil {
			reply <- []*net.UDPAddr{newAddr}
		}
	}
}
