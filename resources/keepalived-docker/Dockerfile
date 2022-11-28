# Support FROM override
ARG BASE_IMAGE=ubuntu:22.04

FROM $BASE_IMAGE
ARG DEBIAN_FRONTEND=noninteractive

COPY sample.keepalived.conf /etc/keepalived/keepalived.conf
COPY manage-keepalived.sh manage-keepalived.sh

RUN apt-get -y update && \
    apt-get -y install keepalived && \
    apt-get -y clean

ENTRYPOINT ["/bin/bash", "manage-keepalived.sh"]
