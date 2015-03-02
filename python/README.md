## Dependencies

- [Python](https://www.python.org) 2.7 or 3.x
- [github.com/tsavola/pyinotify-basic](https://github.com/tsavola/pyinotify-basic)


## Usage

Execute a callback:

	import nameq

	def changed(feature):
		print(feature)

	with nameq.FeatureMonitor(changed):
		...

Alternatively:

	import nameq

	class MyFeatureMonitor(nameq.FeatureMonitor):
		def changed(self, feature):
			print(feature)

	with MyFeatureMonitor():
		...

Consume from a queue:

	try:
		import Queue as queuelib  # Python 2
	except ImportError:
		import queue as queuelib  # Python 3

	import nameq

	queue = queuelib.Queue()
	with nameq.FeatureMonitor(queue.put):
		feature = queue.get()
		...

[Gevent](http://gevent.org) support:

	import gevent.monkey

	gevent.monkey.patch_select()
	gevent.monkey.patch_thread()

	import gevent.queue
	import nameq

	queue = gevent.queue.Queue()
	with nameq.FeatureMonitor(queue.put, StopIteration):
		for feature in queue:
			...
