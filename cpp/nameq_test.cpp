#include "nameq.hpp"

#include <cerrno>
#include <cstdio>
#include <vector>

#include <boost/filesystem/operations.hpp>
#include <boost/filesystem/path.hpp>

#include <poll.h>

using namespace nameq;

#define FEATURE_DIR "../test/features"
#define STATE_DIR   "../test/state"

static void test_context()
{
	const char *feature_name = "cpp";

	boost::filesystem::path p = FEATURE_DIR;
	p /= feature_name;

	boost::filesystem::remove(p);

	{
		FeatureContext context(feature_name, FEATURE_DIR);

		if (!context.set("[1, 2, 3]"))
			throw "FeatureContext::set";

		if (!boost::filesystem::exists(p))
			throw "file was not created";
	}

	if (boost::filesystem::exists(p))
		throw "file was not removed";
}

static void test_monitor()
{
	FeatureMonitor monitor;

	if (monitor.init(STATE_DIR) < 0)
		throw "FeatureMonitor::init";

	while (true) {
		std::vector<Feature> output;

		switch (monitor.read(output)) {
		case -1:
			if (errno == EAGAIN || errno == EINTR)
				break;
			else
				throw "FeatureMonitor::read";

		case 0:
			return;

		case 1:
			for (std::vector<Feature>::const_iterator i = output.begin(); i != output.end(); ++i) {
				const char *status = i->data.empty() ? "off" : "on";
				printf("feature: name=%s host=%s %s\n", i->name.c_str(), i->host.c_str(), status);
			}

			continue;
		}

		struct pollfd pollfd = {
			monitor.fd(),
			POLLIN,
			0
		};

		if (poll(&pollfd, 1, -1) != 1)
			throw "poll";
	}
}

int main()
{
	try {
		test_context();
		test_monitor();
	} catch (const char *call) {
		perror(call);
		return 1;
	}

	return 0;
}
