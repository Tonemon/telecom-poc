# Builds Open5GS from the vendored submodule.
FROM ubuntu:22.04 AS build
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      python3-pip python3-setuptools python3-wheel ninja-build build-essential \
      flex bison git cmake libsctp-dev libgnutls28-dev libgcrypt-dev libssl-dev \
      libidn11-dev libmongoc-dev libbson-dev libyaml-dev libnghttp2-dev \
      libmicrohttpd-dev libcurl4-gnutls-dev libtins-dev libtalloc-dev \
      meson pkg-config ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY vendor/open5gs /src/open5gs
WORKDIR /src/open5gs
RUN meson build --prefix=/usr/local && ninja -C build && ninja -C build install

FROM ubuntu:22.04
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      libsctp1 libgnutls30 libgcrypt20 libssl3 libidn12 libmongoc-1.0-0 libbson-1.0-0 \
      libyaml-0-2 libnghttp2-14 libmicrohttpd12 libcurl3-gnutls libtalloc2 libtins4.0 \
      iproute2 iptables ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /usr/local /usr/local
RUN ldconfig
