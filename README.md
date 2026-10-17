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
- **IPv6** is balanced only when every network has it. Otherwise new IPv6 connections are refused, apps
  fall back to IPv4 at once, and IPv4 is spread over every network (same as desktop net-merge).
- DNS goes to 1.1.1.1 and 8.8.8.8, so it works over every network.

## Install

Every push builds a debug APK: open the latest run under **Actions → build**, download the
`net-merge-debug-apk` artifact, unzip it and install the APK (allow installing from unknown sources).

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
| `app/` | the Android app: `MergeVpnService`, `NetworkTracker`, `MainActivity` |

## Ideas for next steps

- Quick Settings tile to start and stop.
- A switch for the IPv6 mode and a choice of which networks to use.
- Per-app exclusions (apps that need one stable IP, like banking).
- Counting traffic per app.

## License

MIT, see `LICENSE`. Third-party parts: `THIRD-PARTY-NOTICES.md`.
