from __future__ import absolute_import

import json
import logging
import os
import tempfile

try:
	import Queue as queuelib
except ImportError:
	import queue as queuelib

import nameq

log_handler = logging.StreamHandler()
log_handler.setFormatter(logging.Formatter("%(message)s"))

nameq.log.addHandler(log_handler)
nameq.log.setLevel(logging.DEBUG)

log = logging.getLogger("test")
log.addHandler(log_handler)
log.setLevel(logging.DEBUG)

def test():
	statedir = tempfile.mkdtemp(prefix="nameq-python-test-")
	try:
		set_feature(statedir, "a", "10.0.0.1", True)
		set_feature(statedir, "a", "10.0.0.2", True)
		set_feature(statedir, "b", "10.0.0.1", True)
		set_feature(statedir, "b", "10.0.0.3", True)

		queue = queuelib.Queue(10)

		with nameq.FeatureMonitor(queue.put, statedir=statedir):
			log.debug("%s", queue.get())
			log.debug("%s", queue.get())

		while True:
			x = queue.get()
			if not x:
				break

			log.debug("%s", x)
	finally:
		delete_tree(statedir)

def set_feature(statedir, feature, host, value):
	temppath = os.path.join(statedir, ".feature")

	with open(temppath, "w") as f:
		json.dump(value, f)

	dirpath = os.path.join(statedir, "features", feature)

	if not os.path.exists(dirpath):
		os.makedirs(dirpath, 0o755)

	os.rename(temppath, os.path.join(dirpath, host))

def unset_feature(statedir, feature, host, value):
	dirpath = os.path.join(statedir, "features", feature)

	os.remove(os.path.join(dirpath, host))

	try:
		os.rmdir(dirpath)
	except Exception:
		pass

def delete_tree(root):
	for dirpath, dirnames, filenames in os.walk(root, topdown=False):
		for name in filenames:
			os.remove(os.path.join(dirpath, name))
		for name in dirnames:
			os.rmdir(os.path.join(dirpath, name))

if __name__ == "__main__":
	test()
