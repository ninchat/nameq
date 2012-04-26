#!/usr/bin/env python

from datetime import timedelta, datetime
import argparse
import collections
import logging
import os
import random
import re
import rfc822
import signal
import time

import boto.s3.connection
import zmq

if os.environ.get("NAMEQ_NOSYSLOG") == "1":
	logging.basicConfig()
	log = logging.getLogger("nameq")
else:
	log_handler = logging.handlers.SysLogHandler(address="/dev/log")
	log_handler.setFormatter(logging.Formatter("%(name)s: %(levelname)s: %(message)s"))
	log = logging.getLogger("nameq")
	log.addHandler(log_handler)

log.setLevel(logging.DEBUG)

class Node(object):

	def __init__(self, addr, names):
		self.addr  = addr
		self.names = names
		self._str  = " ".join([self.addr] + self.names)

		if self.names:
			log.debug("local address %s with name%s %s",
			          addr, "s" if len(self.names) > 1 else "", ", ".join([repr(n) for n in self.names]))
		else:
			log.debug("local address %s without names", addr)

	def __str__(self):
		return self._str

class S3(object):

	key_re = re.compile(
		r".*/([a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?)=(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$")

	def __init__(self, node, peers, bucket_name, prefix):
		conn = boto.s3.connection.S3Connection(
			os.environ["AWS_ACCESS_KEY_ID"],
			os.environ["AWS_ACCESS_KEY_SECRET"])

		self.node   = node
		self.hosts  = None
		self.peers  = peers
		self.bucket = conn.get_bucket(bucket_name)
		self.prefix = prefix
		self.names  = {}

	def update(self):
		for name in self.node.names:
			keyname = "{}nameq/{}={}".format(self.prefix, name, self.node.addr)
			log.debug("storing S3 key %r", keyname)
			self.bucket.new_key(keyname).set_contents_from_string("")

		addrs  = set()
		names  = {}
		expiry = datetime.utcnow() - timedelta(hours=1)

		for key in self.bucket.list(self.prefix):
			match = self.key_re.match(key.name)
			if match:
				name, _, addr = match.groups()
				if all(int(n) < 256 for n in addr.split(".")):
					addrs.add(addr)
					if name not in self.node.names:
						stamp = datetime.strptime(key.last_modified, "%Y-%m-%dT%H:%M:%S.000Z")
						if stamp > expiry:
							names[name] = addr, stamp
						else:
							log.warning("deleting old S3 key: %r (last modified at %s)", str(key.name), stamp)
							try:
								key.delete()
							except KeyboardInterrupt:
								raise
							except:
								log.exception("S3 delete error")
				else:
					log.error("bad S3 key: %r", key.name)
			elif not key.name.endswith("/"):
				log.error("bad S3 key: %r", key.name)

		self.names = names
		self.hosts.update()
		self.peers.publish(addrs)

class Peers(object):

	name_re = re.compile(r"^[a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?$")
	addr_re = re.compile(r"^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$")

	def __init__(self, node, port, linger):
		self.node    = node
		self.hosts   = None
		self.port    = port
		self.linger  = linger
		self.context = zmq.Context()
		self.names   = {}
		self.sub     = self.context.socket(zmq.SUB)
		self.sub.bind("tcp://*:{}".format(self.port))
		self.sub.setsockopt(zmq.SUBSCRIBE, "")

	def __enter__(self):
		return self

	def __exit__(self, *exc):
		self.close()

	def publish(self, addrs):
		pub = self.context.socket(zmq.PUB)

		try:
			for addr in addrs:
				if addr != self.node.addr:
					log.debug("publishing to %s", addr)
					pub.connect("tcp://{}:{}".format(addr, self.port))

			pub.send(str(self.node))
		finally:
			pub.close(int(self.linger * 1000))

	def receive(self, timeout):
		remain = int(timeout * 1000)
		start  = time.time()

		while True:
			if not self.sub.poll(remain):
				break

			msg = self.sub.recv(zmq.NOBLOCK)
			if msg:
				if self.parse(msg):
					timeout /= 10.0

			remain = int((timeout - (time.time() - start)) * 1000)
			if remain <= 0:
				break

	def parse(self, msg):
		changed  = False
		expedite = False

		stamp  = datetime.utcnow()
		tokens = msg.split()
		if tokens and len(tokens) >= 2:
			addr = tokens[0]
			if self.addr_re.match(addr) and all(int(n) < 256 for n in addr.split(".")):
				for name in tokens[1:]:
					if self.name_re.match(name):
						if name not in self.node.names:
							log.debug("received name %r with address %s", name, addr)
							self.names[name] = addr, stamp
							changed = True
						else:
							log.warning("local name (%s) in received message", name)
							expedite = True
					else:
						log.error("bad hostname in received message: %r", name)
			else:
				log.error("bad IPv4 address in received message: %r", addr)
		else:
			log.error("bad message received: %r", msg)

		if changed:
			self.hosts.update()

		return expedite

	def close(self):
		self.sub.close()
		self.context.term()

class Hosts(object):

	def __init__(self, node, dns, filename, sources):
		self.node     = node
		self.dns      = dns
		self.filename = filename
		self.tempname = filename + ".tmp"
		self.sources  = sources
		self.text     = None

	def update(self):
		combo = collections.defaultdict(list)

		for source in self.sources:
			for name, (addr, stamp) in source.names.iteritems():
				combo[name].append((stamp, addr))

		hosts = collections.defaultdict(list)

		for name, pairs in combo.iteritems():
			pairs.sort()
			_, addr = pairs[-1]
			hosts[addr].append(name)

		hosts = hosts.items()
		hosts.sort()

		text = ""

		for addr, names in hosts:
			names.sort()
			text += "{}\t{}\n".format(addr, " ".join(names))

		if self.text is None or text != self.text:
			log.debug("updating %s", self.filename)

			try:
				with open(self.tempname, "w") as file:
					file.write(text)

				os.rename(self.tempname, self.filename)
			except KeyboardInterrupt:
				raise
			except:
				log.exception("hosts update error")
			else:
				self.text = text
				self.dns.reload()

class Dnsmasq(object):

	def __init__(self, pidfile):
		self.pidfile = pidfile

	def reload(self):
		try:
			with open(self.pidfile) as file:
				pid = int(file.readline().strip())

			os.kill(pid, signal.SIGHUP)
		except KeyboardInterrupt:
			raise
		except:
			log.exception("dnsmasq reload error")

def main():
	parser = argparse.ArgumentParser()
	parser.add_argument("--port",           type=int, default=17105)
	parser.add_argument("--hostsfile",      type=str, default="/var/lib/nameq/hosts")
	parser.add_argument("--dnsmasqpidfile", type=str, default="/var/run/sendsigs.omit.d/network-manager.dnsmasq.pid")
	parser.add_argument("--interval",       type=int, default=60)
	parser.add_argument("--s3prefix",       type=str, default="")
	parser.add_argument("s3bucket",         type=str)
	parser.add_argument("addr",             type=str)
	parser.add_argument("names",            type=str, nargs="*")
	args = parser.parse_args()

	node = Node(args.addr, args.names)

	with Peers(node, args.port, args.interval / 11.0) as peers:
		s3    = S3(node, peers, args.s3bucket, args.s3prefix)
		dns   = Dnsmasq(args.dnsmasqpidfile)
		hosts = Hosts(node, dns, args.hostsfile, (s3, peers))

		peers.hosts = hosts
		s3.hosts    = hosts

		interval = args.interval * 0.9 + args.interval * 0.2 * random.random()
		inited   = False

		while True:
			try:
				s3.update()
				inited = True
			except KeyboardInterrupt:
				raise
			except:
				log.exception("S3 error")

			peers.receive(interval if inited else 1)

if __name__ == "__main__":
	try:
		main()
	except KeyboardInterrupt:
		pass
