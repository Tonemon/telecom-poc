# Provisioning

*Using the custom Go `provisioner` + `telcoctl`.*

This walkthrough explains how subscribers are provisioned in this POC, mapped to how a real operator does it, and records the two crypto/enforcement realities we verified against the vendored Open5GS + srsRAN sources.


## 1. What "provisioning" means (eSIM-as-a-file)

A subscriber's identity + secret keys must be written into **both** sides of the authentication handshake or the mutual authentication (AKA) fails:
- on the operator **subscriber database** (HSS in 4G) side, and
- on the **UE**, with a real SIM/eSIM profile, or in our poc case the software-UE config (`deploy/4g/config/ue.conf`).

In this virtual network the srsue `[usim]` block **is** the eSIM profile, minus the plastic. The keys that must match on both sides: `IMSI`, `Ki`, `OPc`, `MCC/MNC` (PLMN 999/70), algorithm (Milenage).


## 2. Operator-style client/server

Real operators never let an employee write straight into the live subscriber DB and a provisioning system sits in between:

```
Employee (CRM/CSR tool)  ->  Provisioning system (BSS/OSS)  ->  HSS / UDM (subscriber DB)
    telcoctl                     provisioner                        MongoDB
```

- **`provisioner`** (server, container on the `epc` network at `172.22.0.30`). This owns the Milenage crypto and the sole write access to Mongo. REST API, bearer-token authenticated.
- **`telcoctl`** (client, the "employee console"), which talks only to the REST API. It never touches Mongo or the crypto directly.


**Documented simplifications** (POC vs. real): real `Ki` is generated inside a SIM-vendor HSM and never known to a human; eSIM profiles are delivered by an SM-DP+ over the air; provisioning interfaces are mutually authenticated. We do not build an HSM as the server plays the vendor/SM-DP+ role, generates `Ki` with `crypto/rand`, and the API is protected by a single bearer token (`dev-operator-token`, a **dev-only** value set in `deploy/4g/docker-compose.yml`).


## 3. `telcoctl` commands

Every **mutating** command takes `--reason <CODE>` (validated) and optional `--note`. Actions are recorded in an append-only `provisioning_audit` collection (queryable via `telcoctl history <imsi>`).

| Command | Reason codes |
|---|---|
| `issue-sim [--count N]` | `NEW_ACTIVATION` `BATCH` `REPLACEMENT` |
| `add` | `NEW_ACTIVATION` `BATCH` `REPLACEMENT` |
| `remove <imsi>` | (`DEPROVISION`) |
| `suspend <imsi>` | `NON_PAYMENT` `LOST_STOLEN` `FRAUD` `MAINTENANCE` |
| `resume <imsi>` | `PAYMENT_RECEIVED` `RECOVERED` `CLEARED` |
| `set-plan <imsi> --dl 100M --ul 50M` | `UPGRADE` `DOWNGRADE` `PROMOTION` |
| `set-ip <imsi> <ipv4>` | `ENTERPRISE` `M2M` `IOT` |
| `list` / `get <imsi>` / `history <imsi>` | - |

`OTHER` is always accepted (note encouraged). The server runs on the `provisioner` container; to drive it from the host build the binary with `make telcoctl` (drops `./bin/telcoctl`) and set `TELCOCTL_SERVER` / `TELCOCTL_TOKEN`.


## 4. Cryptography: Milenage (AES-128), and why it's fixed

The AKA algorithm must be implemented by **both** the core and the UE, or authentication fails. Verified in the vendored sources:

- Open5GS (`vendor/open5gs/lib/crypt/`): `milenage.c` only, **no TUAK**.
- srsRAN srsue (`vendor/srsRAN_4G/srsue/hdr/stack/upper/usim_base.h`): `auth_algo_milenage` and `auth_algo_xor` only, **no TUAK**. XOR is an insecure test dummy.

So **Milenage (AES-128) is the only real algorithm both sides agree on**. It is not the weak link, as AES-128 has no practical break. We implement Milenage ourselves (`internal/milenage`) and unit-test it against **both** the 3GPP TS 35.208 known-answer vectors and the `wmnsk/milenage` library as an oracle. *The choice to implement this ourselves was also based on the fact that I wanted to learn writing programs with Go :)*

**TUAK** (the modern Keccak/SHA-3-based alternative) is unavailable in this stack; revisit for 5G in a later plan (verify UERANSIM support first).

The provisioner's real security decision is **key generation**: `Ki` is generated with `crypto/rand` (never `math/rand`), `OPc` is computed via Milenage, and `subscriber_status`/`AMF` are set correctly.


## 5. Ciphers: the NAS/AS layer (distinct from the AKA algorithm)

Separate from Milenage, the NAS/AS layer negotiates **ciphering + integrity** at attach. The LTE suites:

| Cipher (EEA) | Integrity (EIA) | Algorithm |
|---|---|---|
| `EEA0` | `EIA0` | null (no protection) |
| `128-EEA1` | `128-EIA1` | SNOW 3G |
| `128-EEA2` | `128-EIA2` | AES |
| `128-EEA3` | `128-EIA3` | ZUC |

This is **stack config, not per-subscriber provisioning data**. `deploy/4g/config/mme.yaml` is set to prefer **AES**:

```yaml
  security:
    integrity_order: [ EIA2, EIA1, EIA0 ]
    ciphering_order: [ EEA2, EEA1, EEA0 ]
```

(Open5GS's default lists `EEA0` first, i.e. null ciphering. We deliberately prefer `EEA2`.)

**How to observe this:** run `make capture-4g`, then open `deploy/4g/pcap/ue_nas.pcap` in Wireshark and find the **NAS Security Mode Command**. The "Selected NAS security algorithms" show `128-EEA2` / `128-EIA2`.


## 6. Suspend: what a real telco does, and what this stack enforces

`suspend` should block a subscriber's service without deleting them. In a real network this is a **Subscriber-Status = OPERATOR_DETERMINED_BARRING** flag in the HSS/UDM: the HSS signals it to the serving MME/AMF (in the Update-Location-Answer, and by pushing an **Insert-Subscriber-Data-Request** or a **Cancel-Location-Request** to detach an active session), and the **MME/AMF enforces** it by detaching and rejecting future attaches with an EMM cause.

**What we verified in Open5GS v2.8.0:**

- The **HSS** faithfully implements the signaling: it stores `subscriber_status`, sends it in the ULA, and (with `use_mongodb_change_stream` enabled + Mongo as a replica set) watches the DB and pushes `IDR`/`CLR` on changes (`request_cancel_location`). The MME honors `CLR` (detach).
- The **MME does NOT read `subscriber_status`** at all (zero references in `src/mme/`). So the barring flag alone is **signaled but not enforced**. A "suspended" UE still attaches and gets an IP.
- **Absence** from the live `subscribers` collection **is** enforced: the HSS returns `USER_UNKNOWN` on the S6a request and the MME rejects the attach (same EMM cause a real ODB reject uses). Verified with the MME running, no restart needed.

**How our `suspend` works (stash-and-restore):** the server **moves** the subscriber document out of the live `subscribers` collection into a `suspended_subscribers` holding collection (all data + keys preserved, `subscriber_status` stamped `1` as honest metadata), so the HSS stops answering for it and attach is blocked. `resume` moves it back. **Nothing is deleted** and it is fully reversible. This is the enforcement leg the MME actually honors; it stands in for the MME's missing `Subscriber-Status` check.

`make test-provisioner-lifecycle` proves it end-to-end: attach + ping, then `suspend` → attach **rejected**, then `resume` → attach + ping restored.

**Why this is the right foundation for the roadmap:** enforcement lives at the **home HSS** via the standard S6a path that **roaming** also uses (a visited MME queries the home HSS; `USER_UNKNOWN` blocks the roamer regardless of whether the visited MME implements barring). It needs no vendored-code patch or replica-set, and it is orthogonal to **X2 handover** (a RAN-side procedure). Future roaming-specific enhancements layer on top without conflict:

- enable `use_mongodb_change_stream` (+ Mongo replica set) so `suspend` also fires a real **CLR** that detaches an active/roaming UE live (observable in pcap);
- patch the vendored Open5GS **MME to honor `Subscriber-Status`** for the authentic on-the-wire ODB-reject semantics.


## 7. Static IP, plan (AMBR), batch

- `set-ip` writes the enforced `slice.0.session.0.ue.ipv4`, the UE then gets that fixed address.
- `set-plan` writes the UE-AMBR (`ambr.downlink/uplink`, value + unit) and the session AMBR, the
  subscriber's data-plan throughput cap.
- `issue-sim --count N` assigns a contiguous IMSI block from PLMN 999/70 (IoT/M2M batch activation).
