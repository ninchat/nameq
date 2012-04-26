## Operation

Each host writes their hostname-to-address mappings to Amazon S3 as empty
files:

    my-bucket/my-prefix/nameq/my-domain-1=10.0.0.1
    my-bucket/my-prefix/nameq/my-domain-2=10.0.0.2
    my-bucket/my-prefix/nameq/my-domain-2=10.0.0.3

The latest file is effective when there are duplicate hostnames.  All S3 keys
under "my-bucket/my-prefix" are loaded, so configuration is also possible via
eg.:

    my-bucket/my-prefix/static/custom.domain=10.0.1.1

New hosts broadcast their addresses to existing hosts (found in S3) via ZeroMQ
(at port 17105 by default).  Local database is kept in a /etc/hosts-like text
file ("/var/lib/nameq/hosts" by default) and dnsmasq is poked whenever it
changes.  (dnsmasq is expected to somehow be configured to use the hosts file.)


## Dependencies

- Python
- ZeroMQ
- Boto
- dnsmasq


## Usage

    nameq.py --s3prefix=my-prefix/ my-bucket $(get-iface-addr-somehow) my-domain-1

