# net-merge for Android

**Use Wi-Fi and mobile data at the same time.** Apps that open many connections (downloads, browsers,
app updates, cloud uploads) get the speed of both networks added up.

An Android port of [net-merge](https://github.com/rohit267/net-merge) by Rohit Mahto. Like the desktop
version there is no server, no account and no root: each new connection is opened on one of your
networks, and faster networks get more of them.

> **Status: early prototype.** It builds and the Go core is tested, but it has not been tried on a
> real phone yet. Expect rough edges.

## How it works

1. The app starts a local VPN. It is only a way to see every app's connections; nothing is sent to
   any VPN server.
2. It asks Android to keep mobile data on while Wi-Fi is connected (normally Android turns it off).
3. Each connection from an app ends in a small TCP/IP stack inside the app (gVisor, through
   tun2socks), and is opened again on Wi-Fi or mobile data. The socket is bound to that network with
   `Network.bindSocket`.
4. net-merge's scheduler picks the network: the least busy one, weighted by measured speed. Networks
   without working internet get nothing until they recover.

```
apps ──► VPN adapter ──► userland TCP/IP (Go) ──► balancer ──┬─► Wi-Fi
                                                             └─► mobile data
```

Details and the differences from desktop net-merge: [docs/PORTING.md](docs/PORTING.md).

## Good to know

- **Mobile data stays on** while merging, so part of your traffic uses your data plan and battery.
- **One connection always uses one network.** A single video stream or single-stream download does
  not get faster. Many parallel connections do.
- **It takes Android's VPN slot**, so it can't run together with another VPN.
- **Some sites tie your login to one IP address** and may ask you to log in again.
- **Apps that don't merge:** every app merges by default. If an app doesn't work or keeps logging you
  out, tick it under *Apps that don't merge*. It then skips net-merge completely and uses one network,
  as if net-merge weren't installed. Changes apply when you leave that screen (open connections
  restart once).
- **Speed on screen:** optional. A small see-through badge appears under the status bar (top right,
  under the battery) only while net-merge carries more than 100 Mbps, and hides below that. It needs
  the "Display over other apps" permission once, and counts only traffic that goes through net-merge.
- **IPv6** is balanced only when every network has it. Otherwise new IPv6 connections are refused, apps
  fall back to IPv4 at once, and IPv4 is spread over every network (same as desktop net-merge).
- DNS goes to 1.1.1.1 and 8.8.8.8, so it works over every network.

## Install

Every push builds the app and publishes it as the **android-latest** release. On your phone, open
[Releases](https://github.com/dewil95/heart/releases/tag/android-latest), download
`net-merge-debug.apk`, open it and allow installing from that source. Builds are signed with the same
debug key, so a newer one installs over the old one.

Open **net-merge**, tap **Start merging** and accept the VPN request. To start it automatically, turn on
*Settings → Network & internet → VPN → net-merge → Always-on VPN*.

## Build it yourself

You need Go (version in `core/go.mod`), JDK 17, the Android SDK and the Android NDK.

```
export ANDROID_HOME=~/Android/Sdk
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/<version>
scripts/build-core.sh          # Go core → app/libs/netmerge-core.aar (gomobile bind)
./gradlew assembleDebug        # → app/build/outputs/apk/debug/app-debug.apk
```

Tests for the Go core need no phone: `cd core && go test ./...`.

## Layout

| Path | What |
|---|---|
| `core/engine` | net-merge's scheduler, balancer and probes (ported) |
| `core/nics` | the network list and socket pinning, through the app |
| `core/tunfd` | userland TCP/IP stack on the VPN's file descriptor |
| `core/mobile` | the API the app calls (`mobile.Mobile` in Java/Kotlin) |
| `app/` | the Android app: `MergeVpnService`, `NetworkTracker`, `MainActivity`, `AppsActivity` (apps that don't merge) |

## Ideas for next steps

- Quick Settings tile to start and stop.
- A switch for the IPv6 mode and a choice of which networks to use.
- Keep each website on one network, so sites that tie logins to an IP still merge across sites.
- Counting traffic per app.

## License

MIT, see `LICENSE`. Third-party parts: `THIRD-PARTY-NOTICES.md`.
