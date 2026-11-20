# Builds srsRAN_4G (with ZMQ RF) from the vendored submodule.
FROM ubuntu:22.04 AS build
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake libfftw3-dev libmbedtls-dev libboost-program-options-dev \
      libconfig++-dev libsctp-dev libzmq3-dev git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY vendor/srsRAN_4G /src/srsRAN_4G
WORKDIR /src/srsRAN_4G
# See docs/4G.md for what this patches and why (T3402 retry on attach-reject
# cause #8). Applied at build time rather than committed as a submodule
# commit: the submodule's origin is the real upstream srsRAN_4G repo, which
# we have no push access to, so a commit made only in the local submodule
# clone would be unreachable from a fresh `git clone --recursive`.
COPY deploy/patches/srsran-nas-t3402.patch /tmp/
# Makes the ZMQ log_trx_timeout device argument actually work (it was parsed
# into the wrong buffer), so the eNBs can log 2 s receive timeouts.
COPY deploy/patches/srsran-zmq-log-trx-timeout.patch /tmp/
# Periodic tracking area updates: stock srsue ignores T3412, so an idle UE never reports in and
# the MME detaches it implicitly (see docs/parts/5-hot-join.md, "pull battery").
COPY deploy/patches/srsran-nas-periodic-tau.patch /tmp/
# The submodule's .git is a gitlink pointing at the parent repo's
# .git/modules (outside this build context), so it's a dangling reference
# here; drop it, it isn't needed to compile.
RUN rm -rf .git && git apply /tmp/srsran-nas-t3402.patch \
    && git apply /tmp/srsran-zmq-log-trx-timeout.patch \
    && git apply /tmp/srsran-nas-periodic-tau.patch
# Build only what the stack runs (srsenb, srsue, the RF loader and its ZMQ plugin). A plain
# `make` also compiles every unit test and example (measured on 8 CPUs: 20 min for the full
# build vs 15 min for this one including the apt layer); srsRAN has no switch for that, and its
# install rules are OPTIONAL, so `cmake --install` installs exactly the targets that were built.
RUN mkdir build && cd build && cmake .. \
    && cmake --build . -j"$(nproc)" --target srsenb srsue srsran_rf srsran_rf_zmq \
    && cmake --install . && ldconfig

FROM ubuntu:22.04
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      libfftw3-single3 libmbedcrypto7 libmbedtls14 libboost-program-options1.74.0 \
      libconfig++9v5 libsctp1 libzmq5 iproute2 iptables iputils-ping ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /usr/local /usr/local
RUN ldconfig
# Tools to use a UE like a phone from a shell: HTTP (curl, wget), DNS (dig, nslookup),
# throughput (iperf3), path (traceroute, mtr, tracepath), raw sockets (nc), and tcpdump to watch
# tun_srsue. Its own layer, so adding a tool never rebuilds srsRAN.
RUN apt-get update && apt-get install -y --no-install-recommends \
      curl wget dnsutils iperf3 traceroute mtr-tiny iputils-tracepath netcat-openbsd tcpdump \
    && rm -rf /var/lib/apt/lists/*
