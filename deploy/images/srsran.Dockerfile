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
# The submodule's .git is a gitlink pointing at the parent repo's
# .git/modules (outside this build context), so it's a dangling reference
# here; drop it, it isn't needed to compile.
RUN rm -rf .git && git apply /tmp/srsran-nas-t3402.patch
RUN mkdir build && cd build && cmake .. && make -j"$(nproc)" && make install && ldconfig

FROM ubuntu:22.04
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      libfftw3-single3 libmbedcrypto7 libmbedtls14 libboost-program-options1.74.0 \
      libconfig++9v5 libsctp1 libzmq5 iproute2 iptables iputils-ping ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /usr/local /usr/local
RUN ldconfig
