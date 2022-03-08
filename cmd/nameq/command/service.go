package command

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ninchat/nameq/service"
)

func serve(prog, command string) (err error) {
	p := service.DefaultParams()

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
		fmt.Fprintf(os.Stderr, "Usage: %s -secretfile=PATH -s3region=REGION -s3bucket=BUCKET [OPTIONS]\n\n", command)
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
	flag.StringVar(&p.Features, "features", p.Features, "features (JSON)")
	flag.StringVar(&p.FeatureDir, "featuredir", p.FeatureDir, "dynamic feature configuration location")
	flag.StringVar(&p.StateDir, "statedir", p.StateDir, "runtime state root location")
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

	err = p.Log.DefaultInit(syslogNet, syslogArg, prog, debug)
	if err != nil {
		println(err.Error())
		return
	}

	secret, err := readFile(secretFd, secretFile)
	if err != nil {
		p.Log.Error(err)
		return
	}

	p.SendMode = &service.PacketMode{
		Secret: secret,
	}

	p.S3Creds, err = readFile(s3CredFd, s3CredFile)
	if err != nil {
		p.Log.Error(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service.HandleSignals(cancel)

	err = service.Serve(ctx, p)
	if err != nil {
		p.Log.Error(err)
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
