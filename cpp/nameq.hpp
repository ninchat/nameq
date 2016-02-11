#ifndef NAMEQ_HPP
#define NAMEQ_HPP

#include <map>
#include <string>
#include <vector>

#ifndef NAMEQ_NOEXCEPT
# include <boost/config.hpp>
# define NAMEQ_NOEXCEPT BOOST_NOEXCEPT
#endif

#ifndef NAMEQ_EXPORT
# ifdef __GNUC__
#  define NAMEQ_EXPORT __attribute__ ((visibility ("default")))
# else
#  define NAMEQ_EXPORT
# endif
#endif

/**
 */
#define NAMEQ_DEFAULT_FEATURE_DIR "/etc/nameq/features"

/**
 */
#define NAMEQ_DEFAULT_STATE_DIR "/run/nameq/state"

/**
 */
namespace nameq {

/**
 * Add or update a local feature.
 *
 * @param name
 * @param data must be a valid JSON document.
 * @param feature_dir
 * @return true on success and false on error.
 */
bool set_feature(const std::string &name, const std::string &data, const char *feature_dir = NAMEQ_DEFAULT_FEATURE_DIR) NAMEQ_NOEXCEPT NAMEQ_EXPORT;

/**
 * Remove a local feature.  Redundant calls are ok.
 *
 * @param name
 * @param feature_dir
 * @return true if it was removed or it didn't exist, and false on error.
 */
bool remove_feature(const std::string &name, const char *feature_dir = NAMEQ_DEFAULT_FEATURE_DIR) NAMEQ_NOEXCEPT NAMEQ_EXPORT;

/**
 * Removes a local feature upon destruction.
 */
class FeatureContext {
public:
	/**
	 * @param name is copied.
	 * @param feature_dir is NOT copied, it must outlive the FeatureContext.
	 */
	explicit FeatureContext(const std::string &name, const char *feature_dir = NAMEQ_DEFAULT_FEATURE_DIR):
		m_name(name),
		m_feature_dir(feature_dir)
	{
	}

	~FeatureContext() NAMEQ_NOEXCEPT
	{
		remove_feature(m_name, m_feature_dir);
	}

	/**
	 * Add or update a local feature.
	 *
	 * @param data must be a valid JSON document.
	 * @return true on success and false on error.
	 */
	bool set(const std::string &data) NAMEQ_NOEXCEPT
	{
		return set_feature(m_name, data, m_feature_dir);
	}

private:
	FeatureContext(const FeatureContext &);
	FeatureContext &operator=(const FeatureContext &);

	const std::string m_name;
	const char *const m_feature_dir;
};

/**
 */
class Feature {
public:
	Feature() NAMEQ_NOEXCEPT
	{
	}

	Feature(const std::string &name, const std::string &host, const std::string &data):
		name(name),
		host(host),
		data(data)
	{
	}

	Feature(const Feature &other):
		name(other.name),
		host(other.host),
		data(other.data)
	{
	}

	Feature &operator=(const Feature &other)
	{
		name = other.name;
		host = other.host;
		data = other.data;
		return *this;
	}

	/**
	 */
	std::string name;

	/**
	 */
	std::string host;

	/**
	 */
	std::string data;
};

/**
 * Usage:
 *
 * 1. init()
 * 2. read() while not empty
 * 3. wait until fd() is readable
 * 4. goto 2 unless you want to stop
 * 5. close()
 */
class FeatureMonitor
{
	typedef std::map<int, std::string> WatchDirs;
	typedef std::map<std::string, int> DirWatches;

public:
	typedef std::vector<Feature> Buffer;

	FeatureMonitor() NAMEQ_NOEXCEPT
	{
	}

	~FeatureMonitor() NAMEQ_NOEXCEPT
	{
		close();
	}

	/**
	 * Must not be called multiple times, unless close is called in between.
	 *
	 * @param state_dir is copied.
	 * @return 0 on success, and -1 on error with errno set.
	 */
	int init(const char *state_dir = NAMEQ_DEFAULT_STATE_DIR) NAMEQ_NOEXCEPT NAMEQ_EXPORT;

	/**
	 * Get pending features updates.
	 *
	 * @param output is appended to.
	 * @return 0 on success, and -1 on error with errno set.
	 */
	int read(Buffer &output) NAMEQ_NOEXCEPT NAMEQ_EXPORT;

	/**
	 * A file descriptor which may be used to wait for feature updates.  Wait
	 * for its readability with select/poll/etc.
	 *
	 * @return a file descriptor if init has been called successfully, and
	 * close hasn't been called yet.
	 */
	int fd() NAMEQ_NOEXCEPT
	{
		return m.fd;
	}

	/**
	 * May be called multiple times, even if init hasn't been called.
	 */
	void close() NAMEQ_NOEXCEPT NAMEQ_EXPORT;

private:
	FeatureMonitor(const FeatureMonitor &);
	FeatureMonitor &operator=(const FeatureMonitor &);

	struct Methods;

	struct Members {
		Members() NAMEQ_NOEXCEPT:
			fd(-1)
		{
		}

		int fd;
		std::string root_dir;
		int root_watch;
		WatchDirs feature_watch_dirs;
		DirWatches feature_dir_watches;
		Buffer buffer;
	};

	Members m;
};

} // namespace nameq

#endif
