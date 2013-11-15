#!/usr/bin/env python

from datetime import timedelta, datetime
import argparse
import collections
import json
import logging
import os
import random
import re
import rfc822
import signal
import time

import boto
import zmq

if os.environ.get("NAMEQ_NOSYSLOG") == "1":
	logging.basicConfig()
	log = logging.getLogger("nameq")
else:
	log_handler = logging.handlers.SysLogHandler(address="/dev/log")
	log_handler.setFormatter(logging.Formatter("%(name)s: %(levelname)s: %(message)s"))
	log = logging.getLogger("nameq")
	log.addHandler(log_handler)

def ordered(sequence):
	return sorted(sequence, key=orderingkey)

def orderingkey(string):
	key = []

	if string:
		tokens = [string[0]]

		for char in string[1:]:
			if bool(tokens[-1].isdigit()) == bool(char.isdigit()):
				tokens[-1] += char
			else:
				tokens.append(char)

		for token in tokens:
			if token.isdigit():
				key.append(int(token))
			else:
				key.append(token)

	return key

class CloseManager(object):

	def __enter__(self):
		return self

	def __exit__(self, *exc):
		self.close()

class Context(CloseManager):

	def __init__(self):
		self._context = zmq.Context()

	def socket(self, *args, **kwargs):
		return self._context.socket(*args, **kwargs)

	def close(self):
		self._context.term()

class Node(object):

	def __init__(self, addr, names):
		self.addr = addr
		self.names = set(names)
		self._str = " ".join([self.addr] + ordered(self.names))

		log.debug("local address %s with names: %s", addr, " ".join(repr(n) for n in self.names))

	def __str__(self):
		return self._str

class S3(object):

	key_re = re.compile(
		r".*/([a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?)=(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$")

	def __init__(self, node, peers, bucket_name, prefix):
		self.node = node
		self.hosts = None
		self.peers = peers
		self.bucket = boto.connect_s3().get_bucket(bucket_name)
		self.prefix = prefix
		self.names = {}

	def update(self):
		for name in self.node.names:
			keyname = "{}nameq/{}={}".format(self.prefix, name, self.node.addr)
			log.debug("storing S3 key %r", keyname)
			self.bucket.new_key(keyname).set_contents_from_string("")

		addrs = set()
		names = {}
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

class Peers(CloseManager):

	name_re = re.compile(r"^[a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?$")
	addr_re = re.compile(r"^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$")

	def __init__(self, context, node, port, linger):
		self.node = node
		self.hosts = None
		self.port = port
		self.linger = linger
		self.context = context
		self.names = {}
		self.sub = self.context.socket(zmq.SUB)
		self.sub.bind("tcp://*:{}".format(self.port))
		self.sub.setsockopt(zmq.SUBSCRIBE, "")

	def publish(self, addrs):
		pub = self.context.socket(zmq.PUB)

		try:
			for addr in addrs:
				if not (addr == self.node.addr or addr.startswith("127.")):
					log.debug("publishing to %s", addr)
					pub.connect("tcp://{}:{}".format(addr, self.port))

			pub.send(str(self.node))
		finally:
			pub.close(int(self.linger * 1000))

	def receive(self, timeout):
		remain = int(timeout * 1000)
		start = time.time()

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
		# legacy format
		if not msg.startswith("{"):
			tokens = msg.split()
			if len(tokens) >= 2:
				self.handle_address({ "address": tokens[0], "names": tokens[1:] })
			else:
				log.error("bad message: %r", msg)
			return

		try:
			doc = json.loads(msg)
		except ValueError:
			log.exception("bad message")
		else:
			if not isinstance(doc, dict):
				log.exception("bad message")
				return

			for key, params in doc.iteritems():
				try:
					handler = getattr(self, "handle_" + key)
				except AttributeError:
					log.warning("no handler for message type: %r", key)
				else:
					try:
						handler(params)
					except:
						log.exception("%s message handling failed", key)

	def handle_address(self, params):
		addr = params["address"]
		names = params["names"]
		changed = False

		if self.addr_re.match(addr) and all(int(n) < 256 for n in addr.split(".")):
			stamp = datetime.utcnow()

			for name in names:
				if self.name_re.match(name):
					if name not in self.node.names:
						log.debug("received name %r with address %s", name, addr)
						self.names[name] = addr, stamp
						changed = True
					else:
						log.warning("local name (%s) in received message", name)
				else:
					log.error("bad hostname in received message: %r", name)
		else:
			log.error("bad IPv4 address in received message: %r", addr)

		if changed:
			self.hosts.update()

	def close(self):
		self.sub.close(0)

class Hosts(object):

	def __init__(self, context, node, dns, hostspath, sources):
		self.node = node
		self.dns = dns
		self.hostspath = hostspath
		self.sources = sources
		self.hosts = None

	def update(self):
		combo = collections.defaultdict(list)

		for source in self.sources:
			for name, (addr, stamp) in source.names.iteritems():
				combo[name].append((stamp, addr))

		hosts = collections.defaultdict(list)
		hosts["127.0.0.1"].extend(self.node.names)

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

		if text != self.hosts:
			if not self.__update(self.hostspath, text):
				return

			if not self.dns.reload():
				return

			self.hosts = text

	@staticmethod
	def __update(path, text):
		log.debug("updating %s", path)

		try:
			temppath = path + ".tmp"

			with open(temppath, "w") as file:
				file.write(text)

			os.rename(temppath, path)
			return True

		except KeyboardInterrupt:
			raise
		except:
			log.exception("%s update error", path)

		return False

class Dnsmasq(object):

	def __init__(self, pidfile):
		self.pidfile = pidfile

	def reload(self):
		try:
			with open(self.pidfile) as file:
				pid = int(file.readline().strip())

			os.kill(pid, signal.SIGHUP)
			return True

		except KeyboardInterrupt:
			raise
		except:
			log.exception("dnsmasq reload error")

		return False

def main(peers_cls=Peers):
	parser = argparse.ArgumentParser()
	parser.add_argument("--port", type=int, default=17105)
	parser.add_argument("--hostsfile", default="/var/lib/nameq/hosts")
	parser.add_argument("--dnsmasqpidfile", default="/var/run/dnsmasq/dnsmasq.pid")
	parser.add_argument("--interval", type=int, default=60)
	parser.add_argument("--s3prefix", default="")
	parser.add_argument("--debug", action="store_true")
	parser.add_argument("s3bucket")
	parser.add_argument("addr")
	parser.add_argument("names", nargs="+")
	args = parser.parse_args()

	log.setLevel(logging.DEBUG if args.debug else logging.INFO)

	node = Node(args.addr, args.names)

	with Context() as context:
		with peers_cls(context, node, args.port, args.interval / 11.0) as peers:
			s3 = S3(node, peers, args.s3bucket, args.s3prefix)
			dns = Dnsmasq(args.dnsmasqpidfile)
			hosts = Hosts(context, node, dns, args.hostsfile, (s3, peers))

			peers.hosts = hosts
			s3.hosts = hosts

			interval = args.interval * 0.9 + args.interval * 0.2 * random.random()
			inited = False

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
