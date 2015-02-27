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
/run/nameq by default) which looks something like this:

	STATEDIR/features/FEATURE-A/10.0.0.1
	STATEDIR/features/FEATURE-A/10.0.0.2
	STATEDIR/features/FEATURE-B/10.0.0.1
	STATEDIR/features/FEATURE-B/10.0.3.4

In other words, information about each host implementing a feature is contained
in a directory named after the feature, and the files are named after the
hosts' IP addresses.  The files contain feature parameters as JSON.  File
creation, modification and removal is atomic; e.g. inotify can be used to
monitor changes in real time.


## Source repository contents

- The top-level and [service](service) directories contain Go sources for the
  nameq program.
- The [go](go) directory contains a Go package for use by application code.


## Dependencies

- Go
- github.com/awslabs/aws-sdk-go
- github.com/miekg/dns
- golang.org/x/exp/inotify


## Building

	$ make


## Usage

	# ./nameq -h

