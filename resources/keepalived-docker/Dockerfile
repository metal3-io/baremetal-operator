# Support FROM override
ARG BASE_IMAGE=ubuntu:22.04

FROM $BASE_IMAGE
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get -y update && \
    apt-get -y install keepalived && \
    apt-get -y clean

COPY sample.keepalived.conf /etc/keepalived/keepalived.conf
COPY manage-keepalived.sh configure-nonroot.sh /bin/

RUN /bin/configure-nonroot.sh && rm /bin/configure-nonroot.sh

CMD ["/bin/bash", "/bin/manage-keepalived.sh"]
