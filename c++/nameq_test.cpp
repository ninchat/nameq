#include "nameq.hpp"

#include <cerrno>
#include <cstdio>
#include <vector>

#include <poll.h>

using namespace nameq;

static void test()
{
	FeatureMonitor monitor;

	if (monitor.init() < 0)
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
		test();
	} catch (const char *call) {
		perror(call);
		return 1;
	}

	return 0;
}
