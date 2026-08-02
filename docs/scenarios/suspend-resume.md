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

Which will now show you 3 IMSI configurations. If you want you can now connect for example to the **UE-3 container** and have internet access through the subscription:
```bash
docker exec -it telecom-poc-4g-ue3-1 /bin/bash

root@87cb3222b2f6:/# ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host 
       valid_lft forever preferred_lft forever
2: eth0@if1516: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether e6:ea:68:f8:61:f8 brd ff:ff:ff:ff:ff:ff link-netnsid 0
    inet 172.22.0.62/24 brd 172.22.0.255 scope global eth0
       valid_lft forever preferred_lft forever
3: tun_srsue: <POINTOPOINT,MULTICAST,NOARP,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UNKNOWN group default qlen 500
    link/none 
    inet 10.45.0.3/24 scope global tun_srsue
       valid_lft forever preferred_lft forever

root@87cb3222b2f6:/# ping -c 3 8.8.8.8
PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=126 time=68.9 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=126 time=63.7 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=126 time=62.4 ms

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2004ms
rtt min/avg/max/mdev = 62.391/64.976/68.860/2.796 ms

```

## Suspend it from the telco side

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl suspend 999700000000003 --reason FRAUD --note he fraudin
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl list

# output
[
 ...
  {
    "imsi": "999700000000003",
    "status": "suspended",
    "dl": "1G",
    "ul": "1G",
    "last_action": "suspend",
    "last_reason": "FRAUD"
  }
 ...
```

And now you will see that the UE-3 has no internet access at all:
```bash
docker exec -it telecom-poc-4g-ue3-1 ping 8.8.8.8
PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 0 received, 100% packet loss, time 2060ms
```

## Resume it

Resuming the subscription is all it takes and the UE reattaches on its own, typically within a few seconds up to about 20-30 seconds. No container restart is needed:

```bash
TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl resume 999700000000003 --reason CLEARED
```

Wait a bit, then confirm the UE has internet access again:
```bash
docker exec -it telecom-poc-4g-ue3-1 ping -c 3 8.8.8.8
PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=126 time=386 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=126 time=54.2 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=126 time=41.8 ms

--- 8.8.8.8 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 41.804/160.678/386.075/159.459 ms
```
