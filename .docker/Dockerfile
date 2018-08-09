FROM centos:6
MAINTAINER Jack Murdock <jack_murdock@comcast.com>

ADD ./caduceus caduceus
RUN mkdir -p /etc/caduceus
VOLUME /etc/caduceus

ENTRYPOINT ./caduceus
