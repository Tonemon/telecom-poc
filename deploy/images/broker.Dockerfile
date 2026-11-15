FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
      gnuradio python3-zmq && rm -rf /var/lib/apt/lists/*
COPY deploy/4g/broker/air.py deploy/4g/broker/api.py deploy/4g/broker/broker.py \
     deploy/4g/broker/world.py deploy/4g/broker/test_air.py deploy/4g/broker/test_api.py \
     deploy/4g/broker/test_world.py /opt/broker/
# Fail the build if the pure air planner regresses.
RUN cd /opt/broker && python3 -m unittest discover -p 'test_*.py'
WORKDIR /opt/broker
ENTRYPOINT ["python3", "broker.py"]
