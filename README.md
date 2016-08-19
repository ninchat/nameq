## Overview

nameq is a peer-to-peer feature discovery system.  It uses Amazon S3 for
persistence and node discovery, and UDP for real-time notifications.  (It
doesn't use IP broadcast/multicast.)

Each peer provides the other peers' configuration information for local
applications.  Feature settings are exposed via a filesystem hierarchy.


## Operation

### Bootstrapping

Each node writes their configuration to S3 in a file named after their IP
address, such as:

	BUCKET/PREFIX/10.0.0.1
	BUCKET/PREFIX/10.0.0.2

A new node scans them in order to find existing nodes.

### Online

Nodes broadcast their configuration to each other via UDP (port 17106 by
default).

S3 is checked once in a while for nodes which may have been missed (e.g. due to
network partition or race condition).  All nodes also participate in cleaning
of old S3 files (left over by dead nodes).


## Configuration

Local features are set either via command line arguments or a config directory.
Files in the config directory are loaded whenever they appear or change.  Their
effects are undone when they disappear.

Files in the config directory (/etc/nameq/features by default) contain feature
parameters (JSON).  The filenames are used as the feature names.  The names may
contain alphanumeric characters (lower or upper case), dashes ("-") and
underscores ("_"); other files are skipped.


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

- The [cmd](cmd) and [service](service) directories contain Go sources for the
  nameq program.
- The [cpp](cpp) directory contains a library for C++ applications.
- The [go](go) directory contains a library for Go applications.
- The [python](python) directory contains a library for Python applications.


## Dependencies

- [Go](https://golang.org)
- [github.com/aws/aws-sdk-go](https://github.com/aws/aws-sdk-go)

See the library directories for C++ and Python dependencies.


## Building

	$ git submodule init
	$ make


## Usage

	$ ./nameq -h


## Dockerization

Build an image:

	$ docker build -t nameq .

Run a container with host network interface:

	$ docker run \
		--name=nameq \
		--net=host \
		--volume=SECRETFILE:/etc/nameq/secret \
		--volume=S3CREDFILE:/etc/nameq/s3creds \
		nameq serve \
			-secretfile=/etc/nameq/secret \
			-s3credfile=/etc/nameq/s3creds \
			-s3region=REGION \
			-s3bucket=BUCKET

Alter local features:

	$ docker run \
		--rm \
		--volumes-from=nameq \
		nameq feature FEATURE true

	$ docker run \
		--rm \
		--volumes-from=nameq \
		nameq monitor-features

