package service

import (
	"bytes"
	"compress/flate"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net"
)

// PacketMode specifies a shared UDP packet configuration.
type PacketMode struct {
	Id     int    // Identifies the configuration.  Must be in range [0..255].
	Secret []byte // The shared HMAC-SHA1 key.
}

var (
	// Preset dictionary used for compressing UDP packets with DEFLATE.
	PacketCompressionDict = []byte("{\"ip_addr\":\",\"time_ns\":,\"names\":[\",\"],\"features\":{\":true,\"}}}")
)

func marshalPacket(local *localNode) (data []byte, err error) {
	var buf bytes.Buffer
	buf.WriteByte(byte(local.mode.Id))

	inflater, err := flate.NewWriterDict(&buf, flate.DefaultCompression, PacketCompressionDict)
	if err != nil {
		return
	}
	if err = local.encodeForPacket(inflater); err != nil {
		return
	}
	inflater.Close()

	mac := hmac.New(sha1.New, local.mode.Secret)
	mac.Write(buf.Bytes())
	buf.Write(mac.Sum(nil))

	data = buf.Bytes()
	return
}

func unmarshalPacket(data []byte, modes map[int]*PacketMode) (node *Node, err error) {
	const modeIdLength = 1
	const digestLength = 20

	messageLength := len(data) - digestLength

	compressedLength := messageLength - modeIdLength
	if compressedLength < 1 {
		err = fmt.Errorf("packet is too short: %d bytes", len(data))
		return
	}

	modeId := int(data[0])

	mode := modes[modeId]
	if mode == nil {
		err = fmt.Errorf("packet has unknown mode: %d", modeId)
		return
	}

	message := data[:messageLength]
	digest := data[messageLength:]

	mac := hmac.New(sha1.New, mode.Secret)
	mac.Write(message)
	if !hmac.Equal(mac.Sum(nil), digest) {
		err = fmt.Errorf("packet is inauthentic (mode %d)", modeId)
		return
	}

	compressed := data[modeIdLength:messageLength]

	deflater := flate.NewReaderDict(bytes.NewBuffer(compressed), PacketCompressionDict)
	defer deflater.Close()

	node = new(Node)
	err = json.NewDecoder(deflater).Decode(node)
	return
}

func verifyPacketOrigin(node *Node, addr *net.UDPAddr) (err error) {
	if ip := net.ParseIP(node.IPAddr); ip == nil {
		err = fmt.Errorf("bad packet address: %s", node.IPAddr)
	} else if !ip.Equal(addr.IP) {
		err = fmt.Errorf("packet address %s doesn't match origin %s", node.IPAddr, addr.IP)
	}
	return
}
