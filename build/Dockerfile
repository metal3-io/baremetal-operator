FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD build/_output/bin/bare-metal-operator /usr/local/bin/bare-metal-operator
