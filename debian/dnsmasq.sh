#!/bin/sh
exec /usr/sbin/dnsmasq.bin --addn-hosts=/var/lib/nameq/hosts "$@"
