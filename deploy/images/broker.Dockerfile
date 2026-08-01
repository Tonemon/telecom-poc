FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
      gnuradio python3-zmq && rm -rf /var/lib/apt/lists/*
COPY deploy/4g/broker/broker.py /opt/broker/broker.py
COPY deploy/4g/broker/test_broker.py /opt/broker/test_broker.py
# Fail the build if the pure planner regresses.
RUN cd /opt/broker && python3 -m unittest test_broker
WORKDIR /opt/broker
ENTRYPOINT ["python3", "broker.py"]
