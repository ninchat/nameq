package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	logging "log"
	"log/syslog"
	"net"
	"os"
)

type Mode struct {
	Id     byte
	Secret []byte
}

type Log struct {
	error *logging.Logger
	info  *logging.Logger
	debug *logging.Logger
}

func (l *Log) Error(args ...interface{}) {
	l.error.Print(args...)
}

func (l *Log) Errorf(fmt string, args ...interface{}) {
	l.error.Printf(fmt, args...)
}

func (l *Log) Info(args ...interface{}) {
	l.info.Print(args...)
}

func (l *Log) Infof(fmt string, args ...interface{}) {
	l.info.Printf(fmt, args...)
}

func (l *Log) Debug(args ...interface{}) {
	if l.debug != nil {
		l.debug.Print(args...)
	}
}

func (l *Log) Debugf(fmt string, args ...interface{}) {
	if l.debug != nil {
		l.debug.Printf(fmt, args...)
	}
}

func main() {
	var (
		ipAddr     = ""
		port       = 17106
		nameArg    = ""
		nameDir    = "/etc/nameq/names"
		featureArg = ""
		featureDir = "/etc/nameq/features"
		stateDir   = "/run/nameq"
		dnsAddr    = "127.0.0.1:53"
		dnsTCP     = true
		dnsUDP     = true
		resolvConf = "/etc/nameq/resolv.conf"
		secretFile = ""
		secretFd   = -1
		s3credFile = ""
		s3credFd   = -1
		s3region   = ""
		s3bucket   = ""
		s3prefix   = ""
		syslogAddr = ""
		syslogNet  = ""
		debug      = false
	)

	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if netAddr, ok := addr.(*net.IPNet); ok && netAddr.IP.IsGlobalUnicast() {
				ipAddr = netAddr.IP.String()
				break
			}
		}
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -addr=IPADDR -secretfile=PATH -s3region=REGION -s3bucket=BUCKET\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "All options:\n")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "The local IP address is guessed if not specified.  The guess may be wrong.\n\n")
		fmt.Fprintf(os.Stderr, "The command-line features specification is a JSON document like this: {\"feature1\":true,\"feature2\":10}\n\n")
		fmt.Fprintf(os.Stderr, "The AWS credentials file should contain two lines of text: an access key id and a secret access key.  They may also be specified via the AWS_ACCESS_KEY and AWS_SECRET_KEY environment variables.\n\n")
		fmt.Fprintf(os.Stderr, "The secret peer-to-peer messaging key is used with HMAC-SHA1.\n\n")
		fmt.Fprintf(os.Stderr, "Log messages are written to stderr unless syslog is enabled.\n\n")
	}

	flag.StringVar(&ipAddr, "addr", ipAddr, "local IP address for peer-to-peer messaging")
	flag.IntVar(&port, "port", port, "UDP port for peer-to-peer messaging")
	flag.StringVar(&nameArg, "names", nameArg, "hostnames (space-delimited list)")
	flag.StringVar(&nameDir, "namedir", nameDir, "dynamic hostname configuration location")
	flag.StringVar(&featureArg, "features", featureArg, "features (JSON)")
	flag.StringVar(&featureDir, "featuredir", featureDir, "dynamic feature configuration location")
	flag.StringVar(&stateDir, "statedir", stateDir, "runtime state root location")
	flag.StringVar(&dnsAddr, "dnsaddr", dnsAddr, "DNS server listening address")
	flag.BoolVar(&dnsTCP, "dnstcp", dnsTCP, "serve DNS clients via TCP?")
	flag.BoolVar(&dnsUDP, "dnsudp", dnsUDP, "serve DNS clients via UDP?")
	flag.StringVar(&resolvConf, "resolvconf", resolvConf, "upstream DNS servers")
	flag.StringVar(&secretFile, "secretfile", secretFile, "path for reading peer-to-peer messaging key")
	flag.IntVar(&secretFd, "secretfd", secretFd, "file descriptor for reading peer-to-peer messaging key")
	flag.StringVar(&s3credFile, "s3credfile", s3credFile, "path for reading AWS credentials")
	flag.IntVar(&s3credFd, "s3credfd", s3credFd, "file descriptor for reading AWS credentials")
	flag.StringVar(&s3region, "s3region", s3region, "S3 region")
	flag.StringVar(&s3bucket, "s3bucket", s3bucket, "S3 bucket")
	flag.StringVar(&s3prefix, "s3prefix", s3prefix, "S3 prefix")
	flag.StringVar(&syslogAddr, "syslog", syslogAddr, "syslog address")
	flag.StringVar(&syslogNet, "syslognet", syslogNet, "remote syslog server network (\"tcp\" or \"udp\")")
	flag.BoolVar(&debug, "debug", debug, "verbose logging")

	flag.Parse()

	if ipAddr == "" || ((secretFile == "") == (secretFd < 0)) || (s3credFile != "" && s3credFd >= 0) || s3region == "" || s3bucket == "" {
		flag.Usage()
		os.Exit(2)
	}

	var err error
	var logWriter io.Writer = os.Stderr

	if syslogAddr != "" {
		if logWriter, err = syslog.Dial(syslogNet, syslogAddr, syslog.LOG_ERR|syslog.LOG_DAEMON, "nameq"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	log := &Log{
		error: logging.New(logWriter, "ERROR: ", 0),
		info:  logging.New(logWriter, "INFO: ", 0),
	}

	if debug {
		log.debug = logging.New(logWriter, "DEBUG: ", 0)
	}

	secret, err := readFile(secretFd, secretFile)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	s3creds, err := readFile(s3credFd, s3credFile)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	mode := &Mode{
		Secret: secret,
	}

	modes := map[byte]*Mode{
		mode.Id: mode,
	}

	local, err := newLocalNode(ipAddr, port, mode)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	remotes := newRemoteNodes(port)

	var (
		notify         = make(chan struct{}, 1)
		notifyState    = make(chan struct{}, 1)
		notifyStorage  = make(chan struct{}, 1)
		notifyTransmit = make(chan struct{}, 1)
		reply          = make(chan []*net.UDPAddr, 10)
	)

	if err := initNameConfig(local, nameArg, nameDir, notify, log); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	if err := initFeatureConfig(local, featureArg, featureDir, notify, log); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	if err = initState(local, remotes, stateDir, notifyState, log); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	if err := initDNS(local, remotes, dnsAddr, dnsTCP, dnsUDP, resolvConf, log); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	go receiveLoop(local, remotes, modes, notifyState, reply, log)
	go transmitLoop(local, remotes, notifyTransmit, reply, log)

	if err := initStorage(local, remotes, notifyStorage, reply, s3creds, s3region, s3bucket, s3prefix, log); err != nil {
		log.Error(err)
		os.Exit(1)
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

func readFile(fd int, path string) (data []byte, err error) {
	if fd >= 0 {
		file := os.NewFile(uintptr(fd), fmt.Sprintf("file descriptor %d", fd))
		defer file.Close()

		data, err = ioutil.ReadAll(file)
	} else if path != "" {
		data, err = ioutil.ReadFile(path)
	}
	return
}
