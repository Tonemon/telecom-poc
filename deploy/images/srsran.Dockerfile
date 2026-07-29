# Builds srsRAN_4G (with ZMQ RF) from the vendored submodule.
FROM ubuntu:22.04 AS build
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake libfftw3-dev libmbedtls-dev libboost-program-options-dev \
      libconfig++-dev libsctp-dev libzmq3-dev git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY vendor/srsRAN_4G /src/srsRAN_4G
WORKDIR /src/srsRAN_4G
RUN mkdir build && cd build && cmake .. && make -j"$(nproc)" && make install && ldconfig

FROM ubuntu:22.04
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      libfftw3-single3 libmbedcrypto7 libmbedtls14 libboost-program-options1.74.0 \
      libconfig++9v5 libsctp1 libzmq5 iproute2 iputils-ping ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /usr/local /usr/local
RUN ldconfig
