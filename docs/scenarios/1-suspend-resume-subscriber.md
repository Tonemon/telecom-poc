# Scenario: suspend and resume a subscriber

*Provision the demo subscribers, watch one get real internet access through the network, then suspend and resume it from the operator side.*

This runs on the multi-UE demo topology (`docs/parts/2-multi-ue-broker.md`) using `telcoctl`, the operator-side provisioning client (`docs/parts/1-provisioning.md`).

## Bring the topology up and provision the demo subscribers

```bash
make 4g-infra
make telcoctl                # Make the provisioning tool if not yet done

make 4g-demo-subscribers
make 4g-demo-devices
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl list
```

Which will now show you 3 IMSI configurations -- and, since `list` merges in the MME's live state, whether each is actually attached right now:
```
IMSI             STATUS  PLAN   NETWORK                  LAST
999700000000001  active  1G/1G  connected (cell 105217)  add/NEW_ACTIVATION
999700000000002  active  1G/1G  connected (cell 105217)  add/NEW_ACTIVATION
999700000000003  active  1G/1G  connected (cell 105474)  add/NEW_ACTIVATION
```

If you want you can now connect for example to the **UE-3 (Samsung 4) container** and have internet access through the subscription:
```bash
docker exec -it telecom-poc-4g-ue3-1 /bin/bash

root@samsung-4:/# ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host 
       valid_lft forever preferred_lft forever
2: eth0@if2198: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether 5e:a4:c7:68:7f:c3 brd ff:ff:ff:ff:ff:ff link-netnsid 0
    inet 172.22.0.63/24 brd 172.22.0.255 scope global eth0
       valid_lft forever preferred_lft forever
3: tun_srsue: <POINTOPOINT,MULTICAST,NOARP,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UNKNOWN group default qlen 500
    link/none 
    inet 10.45.0.5/24 scope global tun_srsue
       valid_lft forever preferred_lft forever

root@samsung-4:/# ping -c 3 8.8.8.8
PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=126 time=62.0 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=126 time=60.3 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=126 time=67.0 ms

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2002ms
rtt min/avg/max/mdev = 60.334/63.120/67.010/2.834 ms

```

## Suspend it from the telco side

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl suspend 999700000000003 --reason FRAUD --note he fraudin
```

Give the live detach a moment to land, then check again. The `samsung-4` UE drops out of the MME's live state entirely, not just its subscriber status:

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl get 999700000000003
```
```
IMSI     999700000000003
Status   suspended (1G/1G)
Network  not registered
Last     suspend / FRAUD
```

And now you will see that `samsung-4` has no internet access at all:
```bash
docker exec -it telecom-poc-4g-ue3-1 ping -c 3 8.8.8.8
PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 0 received, 100% packet loss, time 2045ms
```

## Resume it

Resuming the subscription is all it takes and the UE reattaches on its own, typically within a few seconds up to about a minute. No container restart is needed:

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl resume 999700000000003 --reason CLEARED
```

Wait a bit, then check again for it to be back to `connected`, on a fresh registration (`ue1`/`ue2` show `idle` here, not `connected`: that's normal, they're attached with a live session but just haven't had traffic recently, see `docs/4G.md` §2):

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl list
```
```
IMSI             STATUS  PLAN   NETWORK                  LAST
999700000000001  active  1G/1G  idle (cell 105217)       add/NEW_ACTIVATION
999700000000002  active  1G/1G  idle (cell 105217)       add/NEW_ACTIVATION
999700000000003  active  1G/1G  connected (cell 105474)  resume/CLEARED
```

And confirm the UE has internet access again:
```bash
docker exec -it telecom-poc-4g-ue3-1 /bin/bash

root@samsung-4:/# ping -c 3 8.8.8.8
PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=126 time=52.0 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=126 time=49.1 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=126 time=57.4 ms

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 49.100/52.833/57.400/3.417 ms
```

You will also see that its IP address has changed, which is now `10.45.0.6` instead of `10.45.0.5` and shows a fresh attach, not the old session:
```bash
root@samsung-4:/# ip a
...
3: tun_srsue: <POINTOPOINT,MULTICAST,NOARP,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UNKNOWN group default qlen 500
    link/none 
    inet 10.45.0.6/24 scope global tun_srsue
       valid_lft forever preferred_lft forever
```
