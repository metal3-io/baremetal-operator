# Support FROM override
ARG BASE_IMAGE=ubuntu

FROM $BASE_IMAGE
ARG DEBIAN_FRONTEND=noninteractive

COPY sample.keepalived.conf /etc/keepalived/keepalived.conf
COPY manage-keepalived.sh manage-keepalived.sh

RUN apt-get update -y && \
    apt-get install keepalived -y && \
    apt-get clean

ENTRYPOINT ["/bin/bash", "manage-keepalived.sh"]
