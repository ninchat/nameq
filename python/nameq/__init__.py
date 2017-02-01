"""Application support library for using information exported by nameq."""

from __future__ import absolute_import

__all__ = [
    "DEFAULT_STATEDIR",
    "Feature",
    "FeatureMonitor",
    "log",
    "remove_feature",
    "set_feature",
]

import errno
import fcntl
import json
import logging
import os
import select
import tempfile
import threading

import inotify

DEFAULT_FEATUREDIR = "/etc/nameq/features"
DEFAULT_STATEDIR = "/run/nameq/state"

log = logging.getLogger("nameq")

filename_encoding = "utf-8"


def set_feature(name, value, featuredir=DEFAULT_FEATUREDIR):
    _create_config_file(featuredir, name, json.dumps(value))
    return _FeatureRemover(featuredir, name)


def remove_feature(name, featuredir=DEFAULT_FEATUREDIR):
    _remove_config_file(featuredir, name)


class _FeatureRemover(object):

    def __init__(self, featuredir, name):
        self.featuredir = featuredir
        self.name = name

    def __enter__(self):
        pass

    def __exit__(self, *exc):
        remove_feature(self.name, self.featuredir)


def _create_config_file(dirpath, name, data=""):
    try:
        os.makedirs(dirpath, 0o755)
    except OSError:
        pass

    tmpdirpath = os.path.join(dirpath, ".tmp")

    try:
        os.mkdir(tmpdirpath, 0o700)
    except OSError:
        pass

    with tempfile.NamedTemporaryFile(mode="w", dir=tmpdirpath) as f:
        f.write(data)
        f.flush()

        os.chmod(f.name, 0o664)
        os.rename(f.name, os.path.join(dirpath, name))

        if hasattr(f, "_closer"):
            f._closer.delete = False
        else:
            f.delete = False


def _remove_config_file(dirpath, name):
    try:
        os.remove(os.path.join(dirpath, name))
    except OSError as e:
        if e.errno != errno.ENOENT:
            raise


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
        self._wd_featurepaths = {}
        self._featurename_wds = {}
        self._queued_features = []

        self._fd = inotify.init(inotify.CLOEXEC | inotify.NONBLOCK)
        ok = False
        try:
            flags = inotify.ONLYDIR | inotify.CREATE | inotify.DELETE | inotify.DELETE_SELF
            self._featuredir_wd = inotify.add_watch(self._fd, self._featuredir, flags)
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
                self._add_feature(featurename)

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()

    def close(self):
        log.debug("_FeatureMonitor.close method not implemented")

    def _handle(self, event):
        if event.mask & inotify.CREATE:
            self._add_feature(event.name.decode(filename_encoding))

        if event.mask & inotify.DELETE:
            if event.wd == self._featuredir_wd:
                self._remove_feature(event.name.decode(filename_encoding))
            else:
                self._remove_host(event.wd, event.name.decode(filename_encoding))

        if event.mask & inotify.DELETE_SELF:
            self.close()

        if event.mask & inotify.MOVED_TO:
            self._add_host(event.wd, event.name.decode(filename_encoding))

    def _add_feature(self, name):
        log.debug("adding feature %s", name)

        path = os.path.join(self._featuredir, name)

        try:
            flags = inotify.ONLYDIR | inotify.DELETE | inotify.MOVED_TO
            wd = inotify.add_watch(self._fd, path, flags)
        except Exception:
            log.exception("adding watch for %s", path)
            return

        self._wd_featurepaths[wd] = path
        self._featurename_wds[name] = wd

        try:
            hostnames = os.listdir(path)
        except Exception:
            log.exception("listing %s", path)
            return

        for hostname in hostnames:
            self._add_host(wd, hostname)

    def _remove_feature(self, name):
        log.debug("removing feature %s", name)

        wd = self._featurename_wds[name]
        del self._wd_featurepaths[wd]
        del self._featurename_wds[name]

    def _add_host(self, wd, name):
        path = os.path.join(self._wd_featurepaths[wd], name)

        log.debug("adding host %s", path)

        try:
            f = open(path)
        except Exception:
            log.debug("opening %s", path, exc_info=True)
            return

        try:
            with f:
                data = f.read()
        except Exception:
            log.exception("reading %s", path)
            return

        self._enqueue_feature(path, data)

    def _remove_host(self, wd, name):
        path = os.path.join(self._wd_featurepaths[wd], name)

        log.debug("removing host %s", path)

        if os.access(path, os.F_OK):
            log.debug("%s exists, not removing", path)
            return

        self._enqueue_feature(path)

    def _enqueue_feature(self, path, data=None):
        log.debug("enqueuing %s update", path)

        value = None

        if data is not None:
            try:
                value = json.loads(data)
            except Exception:
                log.exception("decoding JSON data %r", data)
                return

        self._queued_features.append(Feature(
            name=os.path.basename(os.path.dirname(path)),
            host=os.path.basename(path),
            value=value,
        ))


class FeatureMonitor(_FeatureMonitor):
    """Watches the nameq runtime state for changes.  Either the 'changed' method
       must be implemented in a subclass, or a callable must be provided as the
       'changed' parameter.  It will be invoked with a Feature instance, or the
       terminator when the monitor is closed.  The 'booted' method/callable
       will be invoked without parameters after all pre-existing features have
       been delivered.  The state directory must exist.

    """

    _bufsize = 65536

    def __init__(self, changed=None, terminator=None, statedir=DEFAULT_STATEDIR, booted=None):
        super(FeatureMonitor, self).__init__(statedir)

        try:
            self._changed = changed or self.changed
        except AttributeError:
            self._changed = self._changed_not_implemented

        try:
            self._booted = booted or self.booted
        except AttributeError:
            self._booted = self._booted_not_implemented

        self.terminator = terminator

        self._pipe = os.pipe()

        try:
            os.O_CLOEXEC
        except AttributeError:
            pass
        else:
            for fd in self._pipe:
                flags = fcntl.fcntl(fd, fcntl.F_GETFD)
                fcntl.fcntl(fd, fcntl.F_SETFD, flags | os.O_CLOEXEC)

        flags = fcntl.fcntl(self._pipe[0], fcntl.F_GETFL)
        fcntl.fcntl(self._pipe[0], fcntl.F_SETFL, flags | os.O_NONBLOCK)

        self._thread = threading.Thread(target=self._loop)
        self._thread.start()

    def close(self):
        """Stop watching and invoke the callback with the terminator."""

        os.close(self._pipe[1])
        self._thread.join()

    def _changed_not_implemented(self, feature):
        log.debug("FeatureMonitor.changed method not implemented")

    def _booted_not_implemented(self):
        log.debug("FeatureMonitor.booted method not implemented")

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

            try:
                self._booted()
            except Exception:
                log.exception("uncaught exception in FeatureMonitor.booted callback")

            for event in self._iter():
                self._handle(event)
                self._deliver()
        finally:
            self._changed(self.terminator)

    def _deliver(self):
        for feature in self._queued_features:
            try:
                self._changed(feature)
            except Exception:
                log.exception("uncaught exception in FeatureMonitor.changed callback")

        del self._queued_features[:]
