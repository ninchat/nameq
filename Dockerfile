FROM debian

MAINTAINER timo@ninchat.com

ENV DEBIAN_FRONTEND noninteractive

VOLUME /etc/nameq/features
VOLUME /run/nameq/state

ENTRYPOINT ["nameq"]

RUN apt-get update && apt-get -y install ca-certificates && apt-get clean

ADD nameq /usr/local/bin/
