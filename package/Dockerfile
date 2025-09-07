# syntax=docker/dockerfile:1.18.0

FROM registry.suse.com/bci/bci-minimal:15.7

ARG TARGETPLATFORM

RUN if [ "$TARGETPLATFORM" != "linux/amd64" ] && [ "$TARGETPLATFORM" != "linux/arm64" ]; then \
    echo "Error: Unsupported TARGETPLATFORM: $TARGETPLATFORM" && \
    exit 1; \
    fi

ENV ARCH=${TARGETPLATFORM#linux/}

COPY bin/harvester-cloud-provider-${ARCH} /usr/bin/harvester-cloud-provider


CMD ["harvester-cloud-provider"]
