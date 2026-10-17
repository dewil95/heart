# How this port relates to net-merge

The balancing logic is net-merge's, ported from
[rohit267/net-merge](https://github.com/rohit267/net-merge) at commit `12fe8c40` (2026-10-16).
Everything that touches the operating system is new, because Android doesn't let an app do what
the desktop versions do.

## What is the same

`core/engine` is upstream's `internal/engine`, nearly unchanged: weighted least-active-flows
scheduling per address family, throughput-based weights, internet probes per network and family,
failover on link errors, the IPv6 `auto/on/off` policy, and the SOCKS5 front-end (kept for its
tests). `core/tunfd/forward.go` is upstream's `internal/tun/forward.go` (refusing IPv6 at the SYN).
The upstream tests run here too.

## What changed in the engine

- A slot is keyed by interface name as before, but also carries the Android **network handle**
  (`Network.getNetworkHandle()`) instead of an interface index. A new handle behind the same name
  (Wi-Fi joined another network) re-probes the slot.
- Sockets are pinned by `nics.ForceEgress(handle)`, which hands the fd to the app
  (`VpnService.protect` + `Network.bindSocket`). If binding fails the network is gone; the error is
  reported as `ENETUNREACH`, so the existing failover sidelines it and retries elsewhere.
- The source address is bound only when one is given. The Android layer gives none (`0.0.0.0` /
  `::`), so the kernel picks it from the bound network. That covers 464XLAT on IPv6-only mobile
  networks, which have no IPv4 address of their own, and IPv6 privacy addresses.
- `nics.Octets` returns nothing: Android gives apps no per-interface byte counters, so only
  net-merge's own traffic is measured.
- **The app runs with equal weights** (`SetAuto(false)`): each new flow goes to the network with
  the fewest active flows. The desktop's auto weights learn each link's speed from the traffic it
  happens to carry, and on a phone that can't tell a slow network from one that just hasn't been
  busy: one big download on Wi-Fi (one connection, so one network) makes mobile data look 20x
  slower from its background chatter alone, and it then gets almost no new flows.
  `TestChatterIsNotAMeasurement` shows the effect. Least-active alone still adapts: a slower
  network holds its flows longer, so it gets fewer new ones. With equal weights, 8 parallel
  downloads on the emulator ran over both networks (81.9 + 56.6 Mbit/s).
- If auto weights are switched back on, they are tuned for phones: a second of traffic counts as a
  measurement only at 2 Mbit/s or more (desktop: 0.4), the weight floor is 0.2 (desktop: 0.05)
  and an idle link's estimate drifts back up 10%/s (desktop: 3%/s).
  `TestChatterIsNotAMeasurement` covers the chatter case.
- Each network counts failed connections and keeps the last error, shown in the app.
- **Connection racing:** `DialTCP` starts a second attempt on another network when the first
  hasn't connected after `RaceAfter` (1 s); the first to connect wins and the rest are drained.
  tun2socks gives each connect 5 s, so without racing a stalled network cost the app the whole 5 s.
- **Suspect:** a network that loses a race or times out drops from "up" to "checking" and is
  probed on the next tick, instead of at its routine 15 s probe. New flows prefer the networks
  still known to work meanwhile.

## What is new

| Desktop (upstream) | Android (this repo) |
|---|---|
| Creates a TUN (Wintun / Linux TUN / utun) and adds two `/1` routes | `VpnService.Builder` creates the adapter with `0.0.0.0/0` and `::/0` routes and hands over the fd (`core/tunfd`) |
| Lists adapters with netlink / IP Helper / route sockets | `NetworkTracker.kt` follows `ConnectivityManager` callbacks and pushes the list into Go |
| `SO_BINDTOIFINDEX` / `IP_UNICAST_IF` / `IP_BOUND_IF` | `Network.bindSocket` + `VpnService.protect`, and the app excludes itself from its own VPN |
| Every connected adapter is up | Android takes mobile data down while on Wi-Fi; `requestNetwork(TRANSPORT_CELLULAR)` keeps it up |
| Tray icon, `status`, autostart | `MainActivity`, an ongoing notification, Android's always-on VPN setting |
| DNS goes to the adapters' resolvers | The VPN advertises 1.1.1.1 and 8.8.8.8, reachable from every network, and DNS is balanced like other UDP |
