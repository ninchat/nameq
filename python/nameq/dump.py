from __future__ import absolute_import, print_function

import argparse
import json
import random
import sys

import boto

def main():
	parser = argparse.ArgumentParser()
	parser.add_argument("--single", action="store_true", help="print at most one entry (at random)")
	parser.add_argument("s3location", help="s3bucket/s3prefix")
	parser.add_argument("feature", nargs="*", help="feature names")
	args = parser.parse_args()

	filter_features = set(args.feature)

	if "/" in args.s3location:
		bucket, prefix = args.s3location.split("/", 1)
		if prefix and not prefix.endswith("/"):
			prefix += "/"
	else:
		bucket = args.s3location
		prefix = ""

	entries = []
	error = None

	for key in boto.connect_s3().get_bucket(bucket).list(prefix):
		data = key.get_contents_as_string()
		try:
			entry_features = set(json.loads(data).get("features", ()))
		except (TypeError, ValueError, KeyError) as e:
			print("{}: {}".format(key.name, e), file=sys.stderr)
			error = e
		else:
			if not filter_features or filter_features & entry_features:
				entries.append(key.name[len(prefix):])

	if entries:
		if args.single:
			print(random.choice(entries))
		else:
			for entry in entries:
				print(entry)
	elif error:
		raise error

if __name__ == "__main__":
	try:
		main()
	except KeyboardInterrupt:
		sys.exit(1)
