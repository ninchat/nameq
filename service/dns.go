package service

import (
	"net"
	"strings"

	"github.com/miekg/dns"
)

var (
	ipv4local = net.IPv4(127, 0, 0, 1)
)

func initDNS(local *localNode, remotes *remoteNodes, addr string, tcp, udp bool, resolvConf string, log *Log) (err error) {
	if !(tcp || udp) {
		return
	}

	config, err := dns.ClientConfigFromFile(resolvConf)
	if err != nil {
		return
	}

	for _, s := range config.Search {
		dns.HandleFunc(s, func(w dns.ResponseWriter, m *dns.Msg) {
			handleSearch(local, remotes, config, log, w, m)
		})
	}

	dns.HandleFunc(".", func(w dns.ResponseWriter, m *dns.Msg) {
		handleGeneral(local, remotes, config, log, w, m)
	})

	if tcp {
		var l net.Listener

		if l, err = net.Listen("tcp", addr); err != nil {
			return
		}

		go func() {
			panic(dns.ActivateAndServe(l, nil, nil))
		}()
	}

	if udp {
		var pc net.PacketConn

		if pc, err = net.ListenPacket("udp", addr); err != nil {
			return
		}

		go func() {
			panic(dns.ActivateAndServe(nil, pc, nil))
		}()
	}

	return
}

// lookupHost finds a local or remote node name.
func lookupHost(local *localNode, remotes *remoteNodes, name string) net.IP {
	if strings.HasSuffix(name, ".") {
		name = name[:len(name)-1]
	}

	if name == "localhost" || name == "localhost.localdomain" || local.hasName(name) {
		return ipv4local
	} else {
		return remotes.resolve(name)
	}
}

// handleGeneral responds to a non-specific DNS request.
func handleGeneral(local *localNode, remotes *remoteNodes, config *dns.ClientConfig, log *Log, w dns.ResponseWriter, m *dns.Msg) {
	if len(m.Question) == 1 {
		lowerName := strings.ToLower(m.Question[0].Name)

		if ip := lookupHost(local, remotes, lowerName); ip != nil {
			handleLocal(w, m, ip)
			return
		}
	}

	handleRemote(w, m, config, log)
}

// handleSearchs responds to a specific DNS request.
func handleSearch(local *localNode, remotes *remoteNodes, config *dns.ClientConfig, log *Log, w dns.ResponseWriter, m *dns.Msg) {
	if len(m.Question) == 1 {
		lowerName := strings.ToLower(m.Question[0].Name)

		for _, s := range config.Search {
			s += "."

			if strings.HasSuffix(lowerName, s) {
				ip := lookupHost(local, remotes, lowerName[:len(lowerName)-len(s)])
				handleLocal(w, m, ip)
				return
			}
		}
	}

	handleRemote(w, m, config, log)
}

// handleLocal writes an original response.
func handleLocal(w dns.ResponseWriter, m *dns.Msg, ip net.IP) {
	defer w.Close()

	r := new(dns.Msg)

	if ip != nil {
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   m.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: ip,
		}

		r = r.SetReply(m)
		r.Answer = append(r.Answer, rr)
	} else {
		r = r.SetRcode(m, dns.RcodeNameError)
	}

	w.WriteMsg(r)
}

// handleRemote looks up and forwards a response from another DNS server.
func handleRemote(w dns.ResponseWriter, m *dns.Msg, config *dns.ClientConfig, log *Log) {
	defer w.Close()

	for _, s := range config.Servers {
		var c dns.Client

		r, _, err := c.Exchange(m, s+":53")
		if err == nil {
			w.WriteMsg(r)
			return
		}

		log.Error(err)
	}

	r := new(dns.Msg)
	r.SetReply(m)
	w.WriteMsg(r)
}
