# Support FROM override
ARG BASE_IMAGE=ubuntu

FROM $BASE_IMAGE
ENV DEBIAN_FRONTEND=noninteractive
COPY sample.keepalived.conf /etc/keepalived/keepalived.conf
COPY  manage-keepalived.sh manage-keepalived.sh
RUN apt update -y && apt install keepalived -y
ENTRYPOINT ["/bin/bash", "manage-keepalived.sh"]
