from __future__ import absolute_import, print_function

import argparse
import json
import logging
import os
import random
import sys

import boto

log = logging.getLogger("nameq.dump")

def dump_hosts(s3bucket, s3prefix, filter_features=None, single=False):
	if s3prefix and not s3prefix.endswith("/"):
		s3prefix += "/"

	entries = []
	error = None

	for key in boto.connect_s3().get_bucket(s3bucket).list(s3prefix):
		if key.name == s3prefix:
			continue

		data = key.get_contents_as_string()
		try:
			entry_features = set(json.loads(data).get("features", ()))
		except (TypeError, ValueError, KeyError) as e:
			log.error("%s: %s", key.name, e)
			error = e
		else:
			log.info("%s", key.name)
			if not filter_features or filter_features & entry_features:
				entries.append(key.name[len(s3prefix):])

	if not entries and error:
		raise error

	if single:
		entries = [random.choice(entries)]

	return entries

def main():
	parser = argparse.ArgumentParser()
	parser.add_argument("--single", action="store_true", help="print at most one entry (at random)")
	parser.add_argument("s3location", help="s3bucket/s3prefix")
	parser.add_argument("feature", nargs="*", help="feature names")
	args = parser.parse_args()

	if "/" in args.s3location:
		bucket, prefix = args.s3location.split("/", 1)
	else:
		bucket, prefix = args.s3location, ""

	progname = os.path.basename(sys.argv[0])

	log_handler = logging.StreamHandler()
	log_handler.setFormatter(logging.Formatter(progname + ": %(message)s"))

	log.addHandler(log_handler)
	log.setLevel(logging.INFO)

	for entry in dump_hosts(bucket, prefix, set(args.feature), args.single):
		print(entry)

if __name__ == "__main__":
	try:
		main()
	except KeyboardInterrupt:
		sys.exit(1)
