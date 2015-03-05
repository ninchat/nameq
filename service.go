package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	logging "log"
	"log/syslog"
	"os"

	"./service"
)

// Default values for more service.Params.
const (
	DefaultDNSAddr = "127.0.0.1:53"
	DefaultDNSTCP  = true
	DefaultDNSUDP  = true
)

func serve(prog string) (err error) {
	p := &service.Params{
		Addr:       service.GuessAddr(),
		Port:       service.DefaultPort,
		NameDir:    service.DefaultNameDir,
		FeatureDir: service.DefaultFeatureDir,
		StateDir:   service.DefaultStateDir,
		DNSAddr:    DefaultDNSAddr,
		DNSTCP:     DefaultDNSTCP,
		DNSUDP:     DefaultDNSUDP,
		ResolvConf: service.DefaultResolvConf,
	}

	var (
		secretFile string
		secretFd   int = -1
		s3CredFile string
		s3CredFd   int = -1
		syslogArg  string
		syslogNet  string
		debug      bool
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -secretfile=PATH -s3region=REGION -s3bucket=BUCKET [OPTIONS]\n\n", prog)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "The local IP address is guessed if not specified.  The guess may be wrong.\n\n")
		fmt.Fprintf(os.Stderr, "The command-line features specification is a JSON document like this: {\"feature1\":true,\"feature2\":10}\n\n")
		fmt.Fprintf(os.Stderr, "The AWS credentials file should contain two lines of text: an access key id and a secret access key.  They may also be specified via the AWS_ACCESS_KEY and AWS_SECRET_KEY environment variables.\n\n")
		fmt.Fprintf(os.Stderr, "The secret peer-to-peer messaging key is used with HMAC-SHA1.\n\n")
		fmt.Fprintf(os.Stderr, "Log messages are written to stderr unless syslog is enabled.\n\n")
	}

	flag.StringVar(&p.Addr, "addr", p.Addr, "local IP address for peer-to-peer messaging")
	flag.IntVar(&p.Port, "port", p.Port, "UDP port for peer-to-peer messaging")
	flag.StringVar(&p.Names, "names", p.Names, "hostnames (space-delimited list)")
	flag.StringVar(&p.NameDir, "namedir", p.NameDir, "dynamic hostname configuration location")
	flag.StringVar(&p.Features, "features", p.Features, "features (JSON)")
	flag.StringVar(&p.FeatureDir, "featuredir", p.FeatureDir, "dynamic feature configuration location")
	flag.StringVar(&p.StateDir, "statedir", p.StateDir, "runtime state root location")
	flag.StringVar(&p.DNSAddr, "dnsaddr", p.DNSAddr, "DNS server listening address")
	flag.BoolVar(&p.DNSTCP, "dnstcp", p.DNSTCP, "serve DNS clients via TCP?")
	flag.BoolVar(&p.DNSUDP, "dnsudp", p.DNSUDP, "serve DNS clients via UDP?")
	flag.StringVar(&p.ResolvConf, "resolvconf", p.ResolvConf, "upstream DNS servers")
	flag.StringVar(&secretFile, "secretfile", secretFile, "path for reading peer-to-peer messaging key")
	flag.IntVar(&secretFd, "secretfd", secretFd, "file descriptor for reading peer-to-peer messaging key")
	flag.StringVar(&s3CredFile, "s3credfile", s3CredFile, "path for reading AWS credentials")
	flag.IntVar(&s3CredFd, "s3credfd", s3CredFd, "file descriptor for reading AWS credentials")
	flag.StringVar(&p.S3Region, "s3region", p.S3Region, "S3 region")
	flag.StringVar(&p.S3Bucket, "s3bucket", p.S3Bucket, "S3 bucket")
	flag.StringVar(&p.S3Prefix, "s3prefix", p.S3Prefix, "S3 prefix")
	flag.StringVar(&syslogArg, "syslog", syslogArg, "syslog address")
	flag.StringVar(&syslogNet, "syslognet", syslogNet, "remote syslog server network (\"tcp\" or \"udp\")")
	flag.BoolVar(&debug, "debug", debug, "verbose logging")

	flag.Parse()

	if p.Addr == "" || ((secretFile == "") == (secretFd < 0)) || (s3CredFile != "" && s3CredFd >= 0) || p.S3Region == "" || p.S3Bucket == "" {
		flag.Usage()
		os.Exit(2)
	}

	var logWriter io.Writer = os.Stderr

	if syslogArg != "" {
		if logWriter, err = syslog.Dial(syslogNet, syslogArg, syslog.LOG_ERR|syslog.LOG_DAEMON, "nameq"); err != nil {
			return
		}
	}

	log := &p.Log
	log.ErrorLogger = logging.New(logWriter, "ERROR: ", 0)
	log.InfoLogger = logging.New(logWriter, "INFO: ", 0)

	if debug {
		log.DebugLogger = logging.New(logWriter, "DEBUG: ", 0)
	}

	secret, err := readFile(secretFd, secretFile)
	if err != nil {
		log.Error(err)
		return
	}

	p.SendMode = &service.PacketMode{
		Secret: secret,
	}

	p.S3Creds, err = readFile(s3CredFd, s3CredFile)
	if err != nil {
		log.Error(err)
		return
	}

	if err = service.Serve(p); err != nil {
		log.Error(err)
	}
	return
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
