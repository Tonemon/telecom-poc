
## Lecture 11 notes - Cellular Network Architectures

*This markdown file contains the open book exam notes of the Advanced Networking course from which the inspiration to create the poc came.*


### Evolution of Cellular Networks - 1G, 2G, 3G, 4G, 5G

| Generation | Era | Key Technology | Max Data Rate | Focus |
|---|---|---|---|---|
| **1G** | 1980s | AMPS, NMT, TACS | ~10 kbps | Analog voice |
| **2G** | 1990s | D-AMPS, GSM, IS-95 (CDMA) | ~100 kbps | Digital voice, packet |
| **3G** | 2000s | WCDMA/HSPA+, CDMA2000/EV-DO | ~10 Mbps | Mobile data |
| **4G** | 2010s | LTE, LTE-Advanced, Gigabit LTE | ~1 Gbps | Mobile broadband |
| **5G** | 2020s | 5G NR (New Radio) | ~10 Gbps | Unified connectivity fabric |

> **Exam tip:** Each generation roughly 10× increase in data rate. 5G goals over 4G: 10× peak bitrate, 10× decrease in latency, 100× traffic capacity (bps/km²).


### Legacy Network Architectures - 2G, 3G

**Basic Cellular Architecture**
- Network divided into **cells**, each with a **Base Station (BS)**, analogous to 802.11 AP.
- Mobile users attach through BS via **air-interface** (physical + link layer protocol between mobile and BS).
- BSs connect to **Mobile Switching Center (MSC)**, which connects to the public telephone network.

#### 2G Architecture (Voice only)
```
Mobile → BTS → BSC → MSC → Gateway MSC → Public Telephone Network (PSTN)
```
- **Base Transceiver Station (BTS)** - radio hardware at cell tower.
- **Base Station Controller (BSC)** - radio Resource Management (RRM), handovers.
- **Mobile Switching Center (MSC)** - call setup/release, routing, inter-MSC handovers, registration, authentication, location tracking.
	- **Gateway MSC**: Interface to public telephone network.
- Multiple BTSs form a **Base Station System (BSS)** controlled by one BSC.

> **Exam tip:** 2G is **circuit-switched** (voice). The BSC handles radio management; the MSC handles the rest.

#### 3G Architecture (Voice + Data)
Key insight: **data network runs in parallel** with existing voice network, voice core unchanged.

```
Voice path:  Mobile → RNC → MSC → Gateway MSC → Public Telephone Network (PSTN)
Data path:   Mobile → RNC → SGSN → GGSN → Internet
```
- **Radio Network Controller (RNC)** - replaces BSC. RRM, handovers.
	- Radio access network = **Universal Terrestrial Radio Access Network (UTRAN)**.
	- Radio interface: WCDMA, HSPA.
- The GPRS Core network (**GPRS Core Network**):
	- **Service GPRS Support Node (SGSN)** - packet routing, mobility management, authentication, charging.
	- **Gateway GPRS Support Node (GGSN)** - gateway to public Internet.

> **Exam tip:** 3G's key architectural insight, bolt a parallel packet-switched data network onto existing circuit-switched infrastructure. The SGSN is the 3G equivalent of the 4G MME for data.

#### Circuit switching vs. Packet switching

|  | Circuit Switching | Packet Switching |
|---|---|---|
| **Resource reservation** | Dedicated path reserved end-to-end before data flows | No reservation; resources shared on-demand |
| **Bandwidth** | Fixed allocation (idle capacity wasted) | Statistical multiplexing (shared dynamically) |
| **Setup latency** | Call setup required before data | No setup; first packet sent immediately |
| **Per-packet overhead** | None (path already established) | Header on every packet (routing overhead) |
| **Failure resilience/tolerance** | Path breaks → call drops | Packets rerouted around failures |
| **QoS guarantee** | Guaranteed (reserved bandwidth/delay) | Best-effort by default (QoS mechanisms added separately) |
| **Service types** | Single service (e.g., voice only) | Multiple services on one infrastructure |
| **Example** | PSTN (telephone), ISDN | Internet, LTE EPC, 5G |


### 4G - 4G vs Wired, 4G LTE

The solution for wide-area mobile Internet.
- Widespread deployment/use.
	- Peak rates exceed 100 Mbps in Downlink (DL) and 50 Mbps in Uplink (UL).
	- Operating bands: 450 MHz up to 6 GHz.

**4G vs. Wired Internet:**

| Similarities to wired Internet | Differences from wired Internet |
|---|---|
| Edge/core distinction, but both belong to same carrier. | Different wireless link/physical layer. |
| Global cellular network: network of networks. | Mobility as 1st-class service. |
| Uses HTTP, DNS, TCP, UDP, IP, NAT. | User identity via SIM card (IMSI). |
| SDN, tunneling, Ethernet. | Strong "home network" versus roaming on visited nets concept. |
| Data/control plane separation. | Global access, with authentication infrastructure and inter-carrier settlements. |

**4G cellular network directives by International Telecommunication Union (ITU)**
- All-IP packet switched network.
- Need for higher data rates and greater spectral efficiency (up to 100 Mbps for high-mobility mobile access, up to 1 Gbps for low-mobility access).
- Dynamically share and use network resources.
- Smooth handovers across heterogeneous networks, including 2G and 3G networks, small cells (picocells, femtocells), relays, and WLANs.
- High quality of service for multimedia applications.

#### 4G LTE Architecture

Standard: **Long Term Evolution (LTE)** - 3GPP Release 8 onward.
- Architecture: **LTE-SAE** (System Architecture Evolution).
- **Evolved Packet Core (EPC)** - **all-IP** core, no circuit switching.
- Access: **E-UTRAN** (Evolved UTRAN).
- Collectively: **Evolved Packet System (EPS)**.
- Key differences from 3G
	- Fully IP-based - no more MSC, no circuit switching.
	- Unified data+voice over IP (VoLTE).
	- Separation of control plane and user/data plane.
	- Much lower RTT (good for interactive traffic/gaming).

> **Exam tip:** EPC virtualization benefits:
> (1) **Elastic scaling** - spin up/down vMME, vSGW, vPGW on demand (e.g., for stadium events).
> (2) **Faster software updates** - patch network functions without hardware replacement.
> (3) **Cost reduction** - COTS servers replace proprietary appliances via NFV MANO.


### 4G Components - UE, eNode-B, HSS, S-GW, P-GW, MME

```
UE ←→ eNode-B ←→ [S-GW ←→ P-GW] → Internet
                      ↕
                     MME ←→ HSS
```

| Component | Full Name | Function |
|---|---|---|
| **UE** | User Equipment | Mobile device; holds SIM with 64-bit International Mobile Subscriber Identity (IMSI) card. |
| **eNode-B** | Evolved Node-B | Base station at 'edge' of carrier's network, manages radio resources and mobile devices in coverage area ('cell'), coordinates handovers via X2. |
| **HSS** | Home Subscriber Service | DB that stores info about mobile devices ('Subscribers') for which the HSS's network is their 'home network', works with MME for device authentication. |
| **S-GW** | Serving Gateway | Data-plane anchor; tunnels data toward P-GW. |
| **P-GW** | PDN Gateway | Gateway to Internet; provides NAT services and IP address allocation. |
| **MME** | Mobility Management Entity | Device authentication (device-to-network, network-to-device) coordinated with mobile home network HSS, path (tunnel) setup from eNB to P-GW. mobile device management: device handover between cells and tracking/paging device location. |

> **Exam tip:** MME = control plane brain (no user data flows through it). S-GW + P-GW = data plane path. HSS = equivalent of 2G HLR.


**4G EPC Interfaces** - the **Evolved Packet Core (EPC)**
- It is an all-IP core, with no circuit switching. Possible 'network' Interfaces:

| Interface | Between | Purpose |
|---|---|---|
| **S1-MME** | eNB ↔ MME | Control traffic (Non-Access Stratum (NAS) signaling relay), device mobility and connections setups. *Explained in LTE Signaling below.* |
| **S1-U** | eNB ↔ S-GW | Carry user-plane data traffic (web browsing, video streams). |
| **S11** | MME ↔ S-GW | Carry control-plane signaling traffic to manage user sessions, coordinate handovers, and handle data paths (bearers). |
| **S5/S8** | S-GW ↔ P-GW | Control + user plane (S5 = intra-operator, S8 = inter-operator/roaming). |
| **S6a** | MME ↔ HSS | Authentication/subscriber data, and location tracking data using the Diameter protocol. |
| **X2** | eNB ↔ eNB | Inter-base-station handover coordination and inter-cell interference coordination; **bypasses core network entirely**. |
| **SGi** | P-GW ↔ Internet | External network interface. It handles native IP user plane traffic `[0.15]`, serving as the network edge toward the public internet, corporate enterprise intranets, or IP Multimedia Subsystem (IMS) services. |

Network Areas in EPC
- **MME Pool Areas** - multiple MMEs can serve same eNBs (load balancing/redundancy).
- **Tracking Areas** - group of cells, UE location is known at this granularity when idle.
- **S-GW Service Areas** - served by one S-GW.



### LTE Data Plane - OFDMA, SC-FDMA, GTP

Protocol Stack (First Hop: UE ↔ eNode-B)
- Extensive use of tunneling to facilitate mobility.
- New protocols **at the link layer** *are in bold*:

| Layer | Protocol | Function |
|---|---|---|
| IP | IP | Standard IP. |
| **PDCP** | Packet Data Convergence Protocol | Header compression, encryption/decryption. |
| **RLC** | Radio Link Control | Fragmentation/reassembly, reliable data transfer (ARQ). |
| **MAC** | Medium Access Control | Slot requests, error correction (HARQ), scheduling. |
| PHY | Physical Layer | Modulation, transmission. |

For LTE Radio Access network: **OFDMA / SC-FDMA**
- **Downstream (BS → UE): OFDMA** - splits bandwidth into orthogonal subcarriers; minimal inter-channel interference.
	- *Orthogonal: minimal interference between channels.*
- **Upstream (UE → BS): SC-FDMA** - better power efficiency for mobile devices.
- Each active UE allocated one or more **0.5 ms time slots** over **12 subcarriers** = Resource Block.
	- Scheduling algorithm for this is **not standardized**, its up to operator to choose (which present opportunity for optimization).

> **Exam tip:** OFDMA = multiple users share the spectrum simultaneously via orthogonal subcarriers → minimal inter-channel interference. SC-FDMA (uplink) uses single-carrier to reduce PAPR for UE battery life.

**Tunneling in the LTE data plane Packet Core**
- Mobile datagram encapsulated via **GPRS Tunneling Protocol (GTP)** inside UDP → sent to S-GW.
- S-GW re-tunnels datagrams to P-GW.
- Key mobility benefit: when UE moves only the tunnel endpoints change, the IP address stays the same.

> **Exam tip:** **GTP-over-UDP-over-IP tunneling** is the core mechanism **enabling seamless mobility**. All data is tunneled between eNB → S-GW → P-GW.


### LTE Control Plane - RRC, LTE Signaling, RAS, NAS

Protocol Stack consists of new protocols for mobility management, security and authentication.
- **Air interface (UE ↔ eNB):**
	- User plane: IP → PDCP → RLC → MAC → PHY.
	- Control plane: RRC (Radio Resource Control) → PDCP → RLC → MAC → PHY.
- **Packet core:** GTPv2-C (control), GTPv1-U (user), Diameter (HSS interface), S1-AP, X2-AP.

**Radio Resource Control (RRC)**
- Connection establishment and release functions.
- Broadcast of system information.
- RRC connection mobility procedures.
- Paging notification and release.

**LTE Signaling** - messages between user equipment (UE) and LTE network (eNodeB/MME) to manage connections, mobility and data sessions.
- **Radio Access Stratum (RAS)** - signaling between UE and eNB (radio hop).
- **Non-Access Stratum (NAS)** - between UE  and (carried transparently through eNB to) MME.
	- **EMM** (EPS Mobility Management): Manages UE mobility, attach/detach.
	- **ESM** (EPS Session Management): Activate/authenticate/modify/deactivate EPS bearers.


### LTE QoS - ESP Bearers, SDFs, QCI, ARP, GBR, MBR

#### EPS Bearers
- In 4G, QoS is applied at the level of Evolved Packet Service (EPS) bearer.
- A **bearer** - a logical pipe between UE and P-GW with defined QoS parameters, such as data rate, delay, and packet error rate.
- **Service Data Flows (SDFs)** - differentiate traffic flowing between applications on a client and a service.
	- SDFs must be mapped to EPS bearers for QoS treatment.

```
UE ←[Radio Bearer]→ eNB ←[S1 Bearer]→ S-GW ←[S5/S8 Bearer]→ P-GW
|←————————— E-RAB ————————→|
|←——————————————— EPS Bearer ————————————————→|
```

- Bearer Types:

| Type | Bit Rate Guarantee | Use Cases |
|---|---|---|
| **Non-GBR** | No guaranteed minimum bit rate (best-effort). | Email, web browsing, file transfer, P2P. |
| **GBR** | Minimum bit rate guaranteed. | Voice, interactive video, real-time gaming. |

QoS Parameters per Bearer
- **QoS Class Identifier (QCI)** - the value that defines resource type, priority, delay budget, packet error loss rate.
	- Service data flows in an EPS bearer share the same quality of service, specifically the same QCI to ensure that they can be transported the same way.
- **Allocation and Retention Priority (ARP)** - used by call admission control to decide if a bearer request should be accepted/rejected.
	- Also used when networks are overloaded to decide which bearers to release and which can preempt others.
- For GBR bearers additionally:  **Guaranteed Bit Rate (GBR)** the miminum, **Maximum Bit Rate (MBR)** the maximum.
- QCI Table:

| QCI | Type | Priority | Delay | Loss Rate | Example |
|---|---|---|---|---|---|
| 1 | GBR | 2 | 100ms | 10⁻² | Conversational Voice. |
| 2 | GBR | 4 | 150ms | 10⁻³ | Live Video Streaming. |
| 3 | GBR | 3 | 50ms | 10⁻³ | Real-Time Gaming. |
| 5 | Non-GBR | 1 | 100ms | 10⁻⁶ | IMS Signaling. |
| 9* | Non-GBR | 9 | 300ms | 10⁻⁶ | Typically used for default bearer (web, email). |

Default vs. Dedicated Bearers
- **Default EPS bearer** - created at registration; Non-GBR; persists for the session's lifetime; always **QCI 9** (lowest priority, 300 ms delay budget); provides basic internet connectivity.
- **Dedicated EPS bearer** - created on demand for specific services needing different QoS (e.g., VoIP).


### LTE Association and mobility & X2 Handover

**Associating with a Base Station (Initial Attach)**
1. BS broadcasts **primary synchronization signal** every 5ms on all frequencies.
2. UE finds a primary synch signal → locates secondary synch signal on this freq. → reads BS system information broadcast (channel bandwidth, configurations, BS's cellular carrier info).
3. UE selects BS to associate with (preference: home carrier).
4. Authentication and data plane setup follow.

**Mobility Management and Handover Decision**
- LTE Release 8: **Hard handovers** (break-before-make).
- Handover decisions based on: radio link quality, UE capability, QoS requirements, operator policy related aspects.
- UE periodically measures signal quality of nearby cells and reports to serving BS to assist with handover decisions.

**X2 Handover (between eNBs with X2 interface, most common)**
- Preparation
	- Source eNB sends HO Request to target eNB.
	- Target eNB performs admission control (checks whether it has the resources to support the mobile device and its QoS requirements).
	- Target eNB sends HO Response.

- Execution
	- Source eNB sends RRC Reconfiguration (HO command) to UE.
	- UE performs RAN procedures for handover (detaches from old cell, synchronizes with new cell).
	- Data forwarded between eNBs during transition.

- Completion
	- Target eNB sends **Path Switch Request** to MME S-GW.
	- MME signals S-GW to reconfigure S-GW to base station tunnel endpoint to target eNB.
	- Target eNB instructs source eNB to release UE resources.

> **Exam tip:** The **path switch procedure** is key, it moves the S-GW tunnel endpoint without changing the UE's IP address. This is how mobility is transparent to upper layers.

> **Exam tip:** **GTP-over-UDP-over-IP tunneling** provides seamless mobility by decoupling the mobile device's fixed core network IP address from its changing radio point of attachment. As a device moves, the **path switch procedure** updates the tunnel destination dynamically using a Tunnel Endpoint Identifier (TEID), shifting downlink traffic to the new base station without dropping the session.


**Tracking Area vs. Paging Area** *(2026 Q8b)*:
- **Tracking area (TA)** = group of cells; the granularity at which the network knows an **idle** UE's location. The UE only reports location when crossing a TA boundary (**Tracking Area Update**) — saves battery and signaling.
- **Paging area** = the set of cells where the network **broadcasts a paging message** to find the UE when downlink data arrives; equals the UE's registered TA (list), since that is all the network knows.
- One line: tracking = **UE→network** location reporting; paging = **network→UE** searching.
- Trade-off in TA size: large TA → few updates, but paging must sweep many cells; small TA → cheap paging, frequent updates.


### 5G Architecture - application in eMBB, URLLC, mMTC

5G Goals & Key Features
- **10× peak bitrate** (vs. 4G), **10× lower latency**, **100× traffic capacity**.
- **5G NR** (New Radio) two frequency ranges
	- **FR1**: 450 MHz - 6 GHz (sub-6 GHz; better coverage).
	- **FR2**: 24 GHz - 52 GHz (mmWave; higher rates, shorter range ~10-100m).
- Not backwards-compatible with 4G at the radio level.
- 5G Application Areas (The "Triangle"):

| Area | Acronym | Key Requirements | Examples |
|---|---|---|---|
| Enhanced Mobile Broadband | **eMBB** | High data rates, high user density. | Better smartphones, VR, hotspots. |
| Ultra-Reliable Low Latency | **URLLC** | Low latency, high reliability. | Better V2X, industrial IoT, 3D gaming. |
| Massive Machine Type Comms | **mMTC** | Many devices, low power, low rate. | Enabling IoT sensors, smart city. |

> **Exam tip:** eMBB = human broadband. URLLC = safety-critical, latency-sensitive. mMTC = IoT/scale. These map to 3GPP Release 15 first specification (2018).


#### 5G RAN Evolution - C-RAN, BBU Split, O-RAN, 5G network slicing

**Classical/Distributed RAN** - Baseband Unit (BBU), the traditional base station) co-located with Remote Radio Unit (RRU) at tower, with antenna on top, connected via Fiber Optic Cable (CPRI), a proprietary fronthaul.

**Centralized RAN (C-RAN)**
- The BBU pool is located 5-15km away from RRUs.
- Fronthaul carries baseband IQ samples (very high bandwidth required).

> **Exam tip:** C-RAN = BBU functions moved to a central pool, reducing cost and enabling coordination between cells (joint processing, interference management); RRU/RRH remains at the tower.

**C-RAN with BBU Split**
- The BBU is split into **DU** (Distributed Unit, lower MAC/PHY) and **CU** (Centralized Unit, upper RLC/PDCP/RRC).
- Fronthaul (RRU↔DU), Midhaul (DU↔CU), Backhaul (CU↔Core).

**O-RAN (Open RAN)**
- All interfaces are now open and standardized.
- C-RAN with BBU Split components are now: **O-RU** (Open Radio Unit), **O-DU**, **O-CU**.
- Enables multi-vendor deployments, software-defined radio.

> **Exam tip:** O-RAN = disaggregation + openness. Key split options determine fronthaul bandwidth requirements, lower in the stack = more fronthaul bandwidth.


**Network Slicing in 5G** - a single operator running multiple virtual networks on their own physical infrastructure, each optimized for a different service type.
- "A 5G slice is composed of a collection of 5G network functions and specific RAT settings combined for a specific use case or business model."
- Each slice has its own instantiation of core network functions (AMF, SMF, UPF etc.), its own QoS policies, and is fully isolated from the others, but all running on the same physical hardware.
- Example: One operator's physical infrastructure.
	- Slice 1: eMBB (smartphones, optimized for throughput).
	- Slice 2: URLLC (factory automation, optimized for latency).
	- Slice 3: mMTC (IoT sensors, optimized for device density).
	- Slice 4: Emergency services (isolated, high reliability).
- Layers:
	1. Service instance layer (e.g., mobile broadband, healthcare, automotive).
	2. Service support layer (caching, analytics, orchestration, security).
	3. Network slice layer (per-slice CN + access functions).
	4. Resource layer (physical + virtual infrastructure).

*(optional, not in exam) **Network Sharing / RAN Sharing** - concept in which multiple operators share hardware. This is called Mobile Virtual Network Operator (MVNO), existed before 5G and is a commercial/regulatory arrangement rather than a technical slicing mechanism.*

#### 5G Non-Standalone vs. Standalone

5G = 5G RAN + 5G Core
- Core is responsible for non-radio functions like authentication, charging, end-to-end connections and paging.

| Mode | RAN | Core | Notes |
|---|---|---|---|
| **NSA** (Non-Standalone) | 5G NR + LTE | 4G EPC | Fast deployment; limited 5G features. |
| **SA** (Standalone) | 5G NR | 5G Core (NGC) | Full 5G capabilities (slicing, URLLC). |

#### 5G Service-Based Architecture (SBA) table

5G core is completely redesigned as a **microservices/API-based architecture**. Each network function exposes a service interface (Nxxx).

| NF | Full Name | Role | Equivalent in 4G |
|---|---|---|---|
| **AMF** | Access and Mobility Function | Mobility management, NAS termination. | MME (mobility part). |
| **SMF** | Session Management Function | Session/bearer management, IP allocation. | MME (session part) + PGW control. |
| **UPF** | User Plane Function | Packet forwarding, QoS enforcement. | S-GW + P-GW (user plane). |
| **UDM** | Unified Data Management | Subscriber data. | HSS. |
| **AUSF** | Authentication Server Function | Authentication. | HSS (auth part). |
| **PCF** | Policy Control Function | Policy rules. | PCRF. |
| **NSSF** | Network Slice Selection Function | Slice selection. | (new in 5G) |
| **NRF** | NF Repository Function | Service discovery. | (new in 5G) |
| **NEF** | Network Exposure Function | Expose capabilities to 3rd parties. | (new in 5G) |
| **AF** | Application Function | Application-level interaction. | (new in 5G) |

**Interfaces:**
- **N1**: UE ↔ AMF (NAS).
- **N2**: gNB ↔ AMF (control).
- **N3**: gNB ↔ UPF (user data).
- **N4**: SMF ↔ UPF (PFCP - control of user plane).
- **N6**: UPF ↔ Data Network (Internet).
- **Uu**: UE ↔ gNB (radio).

> **Exam tip:** Key 5G architectural shift: **Control-User Plane Separation (CUPS)**. SMF controls the UPF but user data never goes through SMF. AMF replaced MME's mobility functions; SMF handles session management. The SBA uses REST/HTTP/2 over a service bus.

#### 5G Control Plane - User Plane separation

**Control Plane (CP):** AMF, SMF, PCF, UDM, AUSF, NSSF, NRF, NEF, AF.
- Session management, IP allocation, authentication, security, mobility, signaling.

**User Plane (UP):** UPF (+ gNB for radio).
- Packet routing/forwarding, filtering, inspection, QoS enforcement.


### 5G vs 4G QoS model table - PDU session

QoS Flows vs. 4G Bearers
- A QoS Flow is a set of IP flows that are associated with a specific level of QoS requirements, such as latency, throughput, and reliability.

| Concept | 4G | 5G |
|---|---|---|
| QoS identifier | QCI | 5QI (5G QoS Identifier) |
| IP flow grouping | EPS Bearer | QoS Flow |
| End-to-end pipe | EPS Bearer (UE↔P-GW) | PDU Session (UE↔UPF) |

**PDU Session**
- Provides end-to-end connectivity between UE and a data network.
	- Each 5G network slice has its own CN slice + access slice.
	- UE can access up to **8 slices** via distinct PDU sessions (one PDU session = one slice).
	- Enabled by NFV + SDN.
- Contains one or more **QoS Flows** (identified by **QFI**, the QoS Flow Identifier).
- Multiple QoS Flows mapped to Radio Bearers on the air interface.


### 5G Protocol Stack - SDAP, PDCP, RLC, MAC, PHY

The User Plane: UE ↔ gNB:

| Layer | Protocol | Function |
|---|---|---|
| **SDAP** | Service Data Adaptation Protocol | Maps QoS flows to radio bearers; marks QFI. |
| **PDCP** | Packet Data Convergence Protocol | Sequence numbering, header compression, ciphering, integrity, reordering, duplication, retransmission. |
| **RLC** | Radio Link Control | Segmentation/reassembly, duplicate dection, ARQ error correction. |
| **MAC** | Medium Access Control | Multiplexing, scheduling information reporting, priority handling, padding. |
| **PHY** | Physical Layer | Modulation/demodulation, FEC encoding/decoding, HARQ, rate matching, frequency and time synchronization, radio measurement, RF processing. |

> **Exam tip:** SDAP is new in 5G (not in 4G). It handles the mapping of QoS flows to radio bearers, this is the "glue" between the 5G QoS model and the radio layer.


**5G Timeline & Evolution**

| Release | Year | Key Features |
|---|---|---|
| **Rel-15** | 2018 | First official 5G spec; NSA+SA, network slicing, SBA, CUPS, massive MIMO, scalable OFDM. |
| **Rel-16** | 2020 | URLLC enhancements, V2X, industrial IoT. |
| **Rel-17+** | 2022+ | Extended ecosystem, NTN (satellite), RedCap. |
| **5G Advanced (Rel-18+)** | 2024+ | 2nd wave innovations. |
| **6G (Rel-21)** | ~2030 | Next technology leap. |


### 4G EPC vs. 5G Core (NGC) table

| Aspect | 4G EPC | 5G NGC |
|---|---|---|
| Architecture style | Monolithic NFs | Service-Based (microservices) |
| Key control NF | MME | AMF + SMF (split) |
| User plane | S-GW + P-GW | UPF |
| Subscriber DB | HSS | UDM + AUSF |
| QoS unit | EPS Bearer (QCI) | QoS Flow (5QI) |
| Session | EPS Bearer | PDU Session |
| Slicing | Limited | Native (NSSF) |
| Interface type | Point-to-point | Service bus (REST/HTTP/2) |

### X2 vs S1 - Handover types in cellular networks

| Type | Interface Used | When | Involves Core? |
|---|---|---|---|
| **X2 handover** | X2 (eNB↔eNB) | Same MME/S-GW pool | Path switch (S-GW update) |
| **S1 handover** | S1-MME | Different MME/S-GW | Full MME involvement |

**S1 handover - when it occurs instead of X2:**
- X2 interface not configured between source and target eNB (different vendors, different sites).
- UE crosses an **MME boundary** (inter-MME area move) → MME relocation required.
- **Example:** UE moves from an eNB served by MME-1 to an eNB in a different geographic region served by MME-2. Since the UE crosses an MME boundary, the S1 path (via the old MME → new MME) must be used; X2 cannot handle cross-MME mobility.
