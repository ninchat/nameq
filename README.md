## Overview

nameq is a peer-to-peer DNS and feature discovery system.  It uses Amazon S3
for persistence and node discovery, and UDP for real-time notifications.  (It
doesn't use IP broadcast/multicast.)

Each peer provides the other peers' configuration information for local
applications: hostnames can be resolved via a local DNS server, and feature
settings are exported via a filesystem hierarchy.


## Operation

### Bootstrapping

Each node writes their configuration (hostnames and features) to S3 in a file
named after their IP address, such as:

	BUCKET/PREFIX/10.0.0.1
	BUCKET/PREFIX/10.0.0.2

A new node scans them in order to find existing nodes.  If multiple hosts claim
the same hostname, the latest file wins.

### Online

Nodes broadcast their configuration to each other via UDP (port 17106 by
default).  If multiple hosts claim the same hostname, the latest packet wins.

S3 is checked once in a while for nodes which may have been missed (e.g. due to
network partition or race condition).  All nodes also participate in cleaning
of old S3 files (left over by dead nodes).


## Configuration

Local hostnames and features are set either via command line arguments or
config directories.  Files in the config directories are loaded whenever they
appear or change.  Their effects are undone when they disappear.

Filenames in the names directory (/etc/nameq/names by default) correspond to
hostnames.  The hostnames must be valid (according to RFC 1123); other files
are skipped.  File contents are ignored.

Files in the features directory (/etc/nameq/features by default) contain
feature parameters (JSON).  The filenames are used as the feature names.  The
names may contain alphanumeric characters (lower or upper case), dashes ("-")
and underscores ("_"); other files are skipped.


## Feature tree

Network-wide feature configuration is written to a directory tree (rooted at
/run/nameq/state by default) which looks something like this:

	STATEDIR/features/FEATURE-A/10.0.0.1
	STATEDIR/features/FEATURE-A/10.0.0.2
	STATEDIR/features/FEATURE-B/10.0.0.1
	STATEDIR/features/FEATURE-B/10.0.3.4

In other words, information about each host implementing a feature is contained
in a directory named after the feature, and the files are named after the
hosts' IP addresses.  The files contain feature parameters as JSON.  File
creation, modification and removal is atomic, and e.g.
[inotify](https://en.wikipedia.org/wiki/Inotify) can be used to monitor changes
in real time.


## Source repository contents

- The top-level and [service](service) directories contain Go sources for the
  nameq program.
- The [cpp](cpp) directory contains a library for C++ applications.
- The [go](go) directory contains a library for Go applications.
- The [python](python) directory contains a library for Python applications.


## Dependencies

- [Go](https://golang.org)
- [github.com/awslabs/aws-sdk-go](https://github.com/awslabs/aws-sdk-go)
- [github.com/miekg/dns](https://github.com/miekg/dns)
- [golang.org/x/exp/inotify](https://golang.org/x/exp/inotify)

See the library directories for C++ and Python dependencies.


## Building

	$ make


## Usage

	$ ./nameq -h


## Dockerization

Build an image:

	$ docker build -t nameq .

Run a container with host network interface and a copy of the original
resolv.conf:

	$ cp /etc/resolv.conf /etc/nameq/resolv.conf

	$ docker run \
		--name=nameq \
		--net=host \
		--volume=SECRETFILE:/etc/nameq/secret \
		--volume=S3CREDFILE:/etc/nameq/s3creds \
		--volume=/etc/nameq/resolv.conf:/etc/nameq/resolv.conf \
		nameq serve \
			-secretfile=/etc/nameq/secret \
			-s3credfile=/etc/nameq/s3creds \
			-s3region=REGION \
			-s3bucket=BUCKET

	$ echo nameserver 127.0.0.1 > /etc/resolv.conf

Alter local names:

	$ docker run \
		--rm \
		--volumes-from=nameq \
		nameq name HOSTNAME

	$ nslookup HOSTNAME

Alter local features:

	$ docker run \
		--rm \
		--volumes-from=nameq \
		nameq feature FEATURE true

	$ docker run \
		--rm \
		--volumes-from=nameq \
		nameq monitor-features

