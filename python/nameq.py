"""Application support library for using information exported by nameq."""

from __future__ import absolute_import

__all__ = [
	"DEFAULT_STATEDIR",
	"Feature",
	"FeatureMonitor",
	"log",
]

import errno
import fcntl
import json
import logging
import os
import select
import threading

import inotify

DEFAULT_STATEDIR = "/run/nameq/state"

log = logging.getLogger("nameq")

class Feature(object):
	"""Represents a momentary state of a feature on a host.

	   Has properties 'name' (of the feature), 'host' (IPv4 or IPv6 address
	   string of the host where the feature exists) and 'value' (None if
	   feature was removed)."""

	def __init__(self, name, host, value=None):
		self.name = name
		self.host = host
		self.value = value

	def __repr__(self):
		return "Feature(name=%r, host=%r, value=%r)" % (self.name, self.host, self.value)

class _FeatureMonitor(object):

	def __init__(self, statedir=DEFAULT_STATEDIR):
		featuredir = os.path.join(statedir, "features")

		try:
			os.mkdir(featuredir, 0o755)
		except OSError as e:
			if e.errno != errno.EEXIST:
				raise

		self._featuredir = os.path.realpath(featuredir)

		self._queued_features = []

		self._fd = inotify.init(inotify.CLOEXEC|inotify.NONBLOCK)
		ok = False
		try:
			inotify.add_watch(self._fd, self._featuredir, inotify.ONLYDIR|inotify.CREATE|inotify.DELETE|inotify.DELETE_SELF)
			ok = True
		finally:
			if not ok:
				os.close(self._fd)

		try:
			featurenames = os.listdir(self._featuredir)
		except Exception as e:
			log.exception("listing %s", self._featuredir)
		else:
			for featurename in featurenames:
				featurepath = os.path.join(self._featuredir, featurename)
				if self._add_feature(featurepath):
					try:
						hostnames = os.listdir(featurepath)
					except Exception as e:
						log.exception("listing %s", featurepath)
					else:
						for hostname in hostnames:
							self._add_host(os.path.join(featurepath, hostname))

	def __enter__(self):
		return self

	def __exit__(self, *exc):
		self.close()

	def close(self):
		log.debug("_FeatureMonitor.close method not implemented")

	def _handle(self, event):
		if event.mask & inotify.CREATE:
			self._add_feature(event.name)

		if event.mask & inotify.DELETE:
			if os.path.dirname(event.name) == self._featuredir:
				self._remove_feature(event.name)
			else:
				self._remove_host(event.name)

		if event.mask & inotify.DELETE_SELF:
			self.close()

		if event.mask & inotify.MOVED_TO:
			self._add_host(event.name)

	def _add_feature(self, path):
		try:
			inotify.add_watch(self._fd, path, inotify.ONLYDIR|inotify.DELETE|inotify.MOVED_TO)
			return True
		except Exception:
			log.exception("adding watch for %s", path)
			return False

	def _remove_feature(self, path):
		try:
			inotify.rm_watch(self._fd, path)
		except Exception:
			log.exception("removing watch for %s", path)

	def _add_host(self, path):
		try:
			f = open(path)
		except Exception:
			return

		try:
			with f:
				data = f.read()
		except Exception:
			log.exception("reading %s", path)
			return

		self._enqueue_feature(path, data)

	def _remove_host(self, path):
		if os.access(path, os.F_OK):
			return

		self._enqueue_feature(path)

	def _enqueue_feature(self, path, data=None):
		value = None

		if data is not None:
			try:
				value = json.loads(data)
			except Exception:
				log.exception("decoding JSON data %r", data)
				return

		self._queued_features.append(Feature(
			name = os.path.basename(os.path.dirname(path)),
			host = os.path.basename(path),
			value = value,
		))

class FeatureMonitor(_FeatureMonitor):
	"""Watches the nameq runtime state for changes.  Either the 'changed' method
	   must be implemented in a subclass, or a callable must be provided as the
	   'changed' parameter.  It will be invoked with a Feature instance, or the
	   terminator when the monitor is closed.  The state directory must
	   exist."""

	_bufsize = 65536

	def __init__(self, changed=None, terminator=None, statedir=DEFAULT_STATEDIR):
		super(FeatureMonitor, self).__init__(statedir)

		if changed is not None:
			self.changed = changed

		self.terminator = terminator

		self._pipe = os.pipe()

		try:
			os.O_CLOEXEC
		except AttributeError:
			pass
		else:
			for fd in self._pipe:
				flags = fcntl.fcntl(fd, fcntl.F_GETFD)
				fcntl.fcntl(fd, fcntl.F_SETFD, flags|os.O_CLOEXEC)

		flags = fcntl.fcntl(self._pipe[0], fcntl.F_GETFL)
		fcntl.fcntl(self._pipe[0], fcntl.F_SETFL, flags|os.O_NONBLOCK)

		self._thread = threading.Thread(target=self._loop)
		self._thread.start()

	def close(self):
		"""Stop watching and invoke the callback with the terminator."""

		os.close(self._pipe[1])
		self._thread.join()

	def changed(self, feature):
		log.debug("FeatureMonitor.changed method not implemented")

	def _iter(self):
		while True:
			try:
				readable, _, _ = select.select([self._fd, self._pipe[0]], [], [])
			except select.error as e:
				if e.args[0] == errno.EINTR:
					continue
				raise

			if self._fd in readable:
				try:
					buf = os.read(self._fd, self._bufsize)
				except OSError as e:
					if e.errno != errno.EAGAIN:
						raise
				else:
					assert buf
					for event in inotify.unpack_events(buf):
						yield event

			if self._pipe[0] in readable:
				try:
					os.read(self._pipe[0], 1)
				except OSError as e:
					if e.errno != errno.EAGAIN:
						raise
				else:
					break

	def _loop(self):
		try:
			self._deliver()
			for event in self._iter():
				self._handle(event)
				self._deliver()
		finally:
			self.changed(self.terminator)

	def _deliver(self):
		for feature in self._queued_features:
			try:
				self.changed(feature)
			except Exception:
				log.exception("uncaught exception in FeatureMonitor.changed callback")

		del self._queued_features[:]
