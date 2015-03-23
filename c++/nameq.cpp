#include "nameq.hpp"

#include <cassert>
#include <cerrno>
#include <climits>
#include <cstddef>
#include <cstdlib>
#include <fstream>
#include <stdexcept>

#include <unistd.h>
#include <sys/stat.h>
#include <sys/types.h>

#include <sys/inotify.h>

#include <boost/filesystem/operations.hpp>
#include <boost/filesystem/path.hpp>
#include <boost/scoped_array.hpp>
#include <boost/system/error_code.hpp>

namespace fs = boost::filesystem;
namespace sys = boost::system;

namespace {

static int throw_errno(int retval)
{
	if (retval < 0)
		throw errno;

	return retval;
}

static int init_inotify(int flags)
{
	return throw_errno(inotify_init1(flags));
}

static int add_inotify_watch(int fd, const char *pathname, uint32_t mask)
{
	return throw_errno(inotify_add_watch(fd, pathname, mask));
}

} // namespace

namespace nameq {

struct FeatureMonitor::Methods {
	explicit Methods(Members &m) NAMEQ_NOEXCEPT:
		m(m)
	{
	}

	int init(const char *state_dir) NAMEQ_NOEXCEPT
	{
		assert(m.fd < 0);

		int ret_errno;

		try {
			fs::path root_dir = state_dir;
			root_dir /= "features";

			if (mkdir(root_dir.c_str(), 0755) < 0) {
				int e = errno;

				if (!fs::exists(root_dir))
					throw e;
			}

			root_dir = fs::canonical(root_dir);
			m.root_dir = root_dir.native();

			m.fd = init_inotify(IN_NONBLOCK|IN_CLOEXEC);
			m.root_watch = add_inotify_watch(m.fd, m.root_dir.c_str(), IN_ONLYDIR|IN_CREATE|IN_DELETE|IN_DELETE_SELF);

			for (fs::directory_iterator i(root_dir); i != fs::directory_iterator(); ++i) {
				add_feature(i->path());

				for (fs::directory_iterator j(i->path()); j != fs::directory_iterator(); ++j)
					add_host(j->path());
			}

			return 0;

		} catch (const sys::system_error &e) {
			ret_errno = e.code().value();
		} catch (const std::bad_alloc &) {
			ret_errno = ENOMEM;
		} catch (int e) {
			ret_errno = e;
		} catch (...) {
			ret_errno = EINVAL;
		}

		close();

		errno = ret_errno;
		return -1;
	}

	int read(Buffer &output) NAMEQ_NOEXCEPT
	{
		assert(m.fd >= 0);

		try {
			while (true) {
				char data[sizeof (inotify_event) + NAME_MAX + 1];

				ssize_t len = ::read(m.fd, data, sizeof (data));
				if (len < 0) {
					if (errno == EAGAIN || errno == EINTR) {
						if (!m.buffer.empty())
							break;
					}

					return -1;
				}

				size_t offset = 0;

				while (offset + sizeof (inotify_event) <= size_t(len)) {
					const inotify_event *event = reinterpret_cast <inotify_event *> (data + offset);
					offset += sizeof (inotify_event) + event->len;

					if (event->mask & IN_CREATE) {
						fs::path path = m.root_dir;
						path /= event->name;

						add_feature(path);
					}

					if (event->mask & IN_DELETE) {
						if (event->wd == m.root_watch) {
							fs::path path = m.root_dir;
							path /= event->name;

							remove_feature(path);
						} else {
							WatchDirs::const_iterator i = m.feature_watch_dirs.find(event->wd);
							if (i != m.feature_watch_dirs.end()) {
								fs::path path = i->second;
								path /= event->name;

								remove_host(path);
							}
						}
					}

					if (event->mask & IN_DELETE_SELF) {
						errno = ENOENT;
						return -1;
					}

					if (event->mask & IN_MOVED_TO) {
						WatchDirs::const_iterator i = m.feature_watch_dirs.find(event->wd);
						if (i != m.feature_watch_dirs.end()) {
							fs::path path = i->second;
							path /= event->name;

							add_host(path);
						}
					}
				}
			}

			if (m.buffer.empty())
				return 0;

			output.reserve(output.size() + m.buffer.size());

			for (Buffer::const_iterator i = m.buffer.begin(); i != m.buffer.end(); ++i)
				output.push_back(*i);

			m.buffer.clear();

			return 1;

		} catch (const std::bad_alloc &) {
			errno = ENOMEM;
		} catch (int e) {
			errno = e;
		} catch (...) {
			errno = EINVAL;
		}

		return -1;
	}

	void close() NAMEQ_NOEXCEPT
	{
		if (m.fd >= 0) {
			// the watches are closed automatically with fd
			::close(m.fd);
			m.fd = -1;
			m.root_dir.clear();
			m.root_watch = -1;
			m.feature_watch_dirs.clear();
			m.feature_dir_watches.clear();
			m.buffer.clear();
		}
	}

	void add_feature(const fs::path &path)
	{
		int watch = add_inotify_watch(m.fd, path.c_str(), IN_ONLYDIR|IN_DELETE|IN_MOVED_TO);
		m.feature_watch_dirs[watch] = path.native();
		m.feature_dir_watches[path.native()] = watch;
	}

	void remove_feature(const fs::path &path)
	{
		DirWatches::iterator i = m.feature_dir_watches.find(path.native());
		if (i != m.feature_dir_watches.end()) {
			int watch = i->second;
			m.feature_watch_dirs.erase(watch);
			m.feature_dir_watches.erase(i);
		}
	}

	void add_host(const fs::path &path)
	{
		std::ifstream stream(path.c_str());
		if (stream) {
			stream.seekg(0, stream.end);
			int size = stream.tellg();
			stream.seekg(0, stream.beg);

			boost::scoped_array<char> data(new char[size]);
			stream.read(data.get(), size);
			if (stream)
				append_feature(path, std::string(data.get(), size));
		}
	}

	void remove_host(const fs::path &path)
	{
		std::ifstream stream(path.c_str());
		if (!stream)
			append_feature(path, std::string());
	}


	void append_feature(const fs::path &path, const std::string &data)
	{
		fs::path name = path.parent_path().filename();
		fs::path host = path.filename();

		m.buffer.push_back(Feature(name.native(), host.native(), data));
	}

	Members &m;
};

int FeatureMonitor::init(const char *state_dir) NAMEQ_NOEXCEPT
{
	return Methods(m).init(state_dir);
}

int FeatureMonitor::read(Buffer &output) NAMEQ_NOEXCEPT
{
	return Methods(m).read(output);
}

void FeatureMonitor::close() NAMEQ_NOEXCEPT
{
	Methods(m).close();
}

} // namespace nameq
