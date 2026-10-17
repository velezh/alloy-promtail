# syntax=docker/dockerfile:1.23@sha256:2780b5c3bab67f1f76c781860de469442999ed1a0d7992a5efdf2cffc0e3d769

# NOTE: This Dockerfile can only be built using BuildKit, with the repository
# root as the build context:
#
#   docker buildx build -f promtail/Dockerfile .
#
# The default target is the runtime image; --target binaries --output
# type=local,dest=<dir> exports the binary only.

ARG UBUNTU_RELEASE=24.04
ARG CHISEL_VERSION=v1.4.2

FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.10.0@sha256:40fe21f44bb8cddf1eb1226db190d0a984118f62ee02a91b77d99448d28264a6 AS xx

# Checksums for each asset are available at GitHub release page: https://github.com/canonical/chisel/releases
FROM scratch AS chisel-amd64
ARG CHISEL_VERSION
ADD --checksum=sha256:3b3d86ea38045e54a13334e358ab8da12e6dd33342163c5c1ba13525f1070cfe --unpack \
    "https://github.com/canonical/chisel/releases/download/${CHISEL_VERSION}/chisel_${CHISEL_VERSION}_linux_amd64.tar.gz" /

FROM scratch AS chisel-arm64
ARG CHISEL_VERSION
ADD --checksum=sha256:398050085cf32d7718ba1e2fad144b179e0943d0e0376b2a0614577c51d331e8 --unpack \
    "https://github.com/canonical/chisel/releases/download/${CHISEL_VERSION}/chisel_${CHISEL_VERSION}_linux_arm64.tar.gz" /

# Chisel cuts the rootfs of any target architecture on the build platform, so
# multi-platform builds need no emulation.
FROM chisel-${BUILDARCH} AS chisel

# Bookworm's glibc 2.36 keeps the binary compatible with the glibc 2.39 of the
# runtime image and with older distributions running the exported binary.
FROM --platform=$BUILDPLATFORM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build

COPY --from=xx / /

ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

# file is required by xx-verify. libsystemd-dev provides the headers of
# loki.source.journal, which loads libsystemd at runtime.
RUN apt-get update \
    && apt-get install -qy --no-install-recommends file \
    && xx-apt-get install -qy --no-install-recommends gcc libc6-dev libsystemd-dev

ARG VERSION
ARG GIT_REVISION
ARG GIT_BRANCH
ARG BUILD_DATE
ARG BUILD_USER=docker
ARG COMPONENT_GROUPS

# xx-go targets TARGETPLATFORM; GOOS and GOARCH are passed as well because the
# Makefile defaults them to the build platform.
RUN --mount=type=bind,target=/src \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make -C /src/promtail build \
        GO=xx-go GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" CGO_ENABLED=1 \
        BUILD_DIR=/tmp/promtail BINARY=/out/alloy-promtail \
        VERSION="${VERSION}" GIT_REVISION="${GIT_REVISION}" GIT_BRANCH="${GIT_BRANCH}" \
        BUILD_DATE="${BUILD_DATE}" BUILD_USER="${BUILD_USER}" \
        COMPONENT_GROUPS="${COMPONENT_GROUPS}" \
    && xx-verify /out/alloy-promtail

FROM scratch AS binaries

COPY --from=build /out/alloy-promtail /alloy-promtail

FROM --platform=$BUILDPLATFORM public.ecr.aws/ubuntu/ubuntu:noble@sha256:748740465d0aadaa69ab6e6c295892f17d7a8f44a85090dbb571ec0bb8c5674f AS slicer

COPY --from=xx / /

# Username and uid for alloy user
ARG UID="473"
ARG USERNAME="alloy"
ARG UBUNTU_RELEASE
ARG TARGETPLATFORM
SHELL ["/bin/bash", "-oeux", "pipefail", "-c"]

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -qy --no-install-recommends ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY --from=chisel /chisel /usr/bin/chisel
# base-files_chisel generates the Chisel manifest. Keep the package cache until
# its Debian control records have populated the dpkg status file for scanners.
RUN mkdir -p /rootfs/var/lib/dpkg \
    && XDG_CACHE_HOME=/tmp/chisel-cache chisel cut --release "ubuntu-${UBUNTU_RELEASE}" \
        --arch "$(xx-info debian-arch)" --root /rootfs \
        base-files_base \
        base-files_chisel \
        base-files_release-info \
        ca-certificates_data \
        libc6_libs \
        libsystemd0_libs \
        tzdata_zoneinfo \
    && for archive in /tmp/chisel-cache/chisel/sha256/*; do \
        if dpkg-deb --field "$archive" Package >/dev/null 2>&1; then \
            dpkg-deb --field "$archive" >> /rootfs/var/lib/dpkg/status; \
            printf '\n' >> /rootfs/var/lib/dpkg/status; \
        fi; \
    done \
    && rm -rf /tmp/chisel-cache

# Create the Alloy user directly because the chiseled rootfs has no useradd.
RUN cat >>/rootfs/etc/passwd <<EOF
root:x:0:0:root:/root:/sbin/nologin
${USERNAME}:x:${UID}:${UID}::/home/${USERNAME}:/sbin/nologin
EOF
RUN cat >>/rootfs/etc/group <<EOF
root:x:0:
${USERNAME}:x:${UID}:
EOF

RUN mkdir -p /rootfs/etc/alloy /rootfs/var/lib/alloy/data /rootfs/home/${USERNAME} \
    && chown -R ${UID}:${UID} /rootfs/var/lib/alloy /rootfs/home/${USERNAME} \
    && chmod -R 770 /rootfs/var/lib/alloy

###

FROM scratch

LABEL org.opencontainers.image.source="https://github.com/grafana/alloy" \
      org.opencontainers.image.title="alloy-promtail" \
      org.opencontainers.image.description="Grafana Alloy distribution for log collection" \
      org.opencontainers.image.licenses="Apache-2.0"

# The binary and the config stay owned by root, so the alloy user cannot
# modify them.
COPY --from=slicer /rootfs/ /
COPY --from=build /out/alloy-promtail /usr/bin/alloy-promtail
COPY --chmod=644 promtail/example-config.alloy /etc/alloy/config.alloy

# root stays the default user because host logs mounted into the container are
# usually readable by root only; run with --user 473:473 when they are not.
ENTRYPOINT ["/usr/bin/alloy-promtail"]
ENV ALLOY_DEPLOY_MODE=docker
CMD ["run", "/etc/alloy/config.alloy", "--storage.path=/var/lib/alloy/data"]
