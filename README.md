# telecom-poc

A proof-of-concept, fully-virtual 4G/5G mobile network built from open-source components (Open5GS + srsRAN_4G + UERANSIM), for learning telecom internals. No RF hardware, no over-the-air transmission. See `docs/` for `THEORY.md` background, walkthroughs, and more.


## Quickstart (4G)

```bash
make submodules      # fetch Open5GS / srsRAN_4G / UERANSIM / PacketRusher
make up-4g           # build + start the 4G core and RAN
make status-4g       # check health
make down-4g         # tear down
```

## Layout

- `deploy/` - Docker Compose stacks and configs
- `tools/` - custom Go tools (provisioner, loadgen)
- `vendor/` - upstream projects as git submodules
- `docs/` - theory, walkthroughs, and more
