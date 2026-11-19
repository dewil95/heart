# net-merge for Windows

Two small programs that put apps on the network **you** choose. No driver, no
admin rights, no DLL injection — a single `.exe` each, pure Go standard library.

This is the opposite of the Android app. The Android app *merges* one app's
traffic across both networks for speed. These *separate* apps onto different
networks. (Windows has no clean, driver-free way to merge one app across links;
separating is the part that works reliably.)

## chrome-on-wifi — the one-click one

Opens **one Chrome that uses Wi-Fi**. Everything else on the PC keeps using its
normal connection (the cable, when a cable is the default). No config file.

1. Download `chrome-on-wifi-windows-x64.exe` from the **windows-latest** release
   (use the arm64 file on a Snapdragon/ARM laptop). Put it in its own folder,
   e.g. `C:\netmerge`.
2. Double-click it. SmartScreen may warn (the file isn't signed): **More info →
   Run anyway**.
3. A separate Chrome window opens — that one is on Wi-Fi, with its own logins.
   Your usual Chrome and everything else stay on the cable. Keep the little black
   window open; closing it (or that Wi-Fi Chrome) stops it.

Check it worked: in the Wi-Fi Chrome open a "what is my IP" page; it shows your
Wi-Fi line's IP, different from the rest of the PC when the two use different
internet lines.

## chrome-merge — use both networks at once

Opens **one Chrome that uses all your networks together** (cable + Wi-Fi). It
spreads Chrome's connections across every network that has internet, sending each
new one to the least-busy network. Pages with many parts, and multi-part
downloads, get the combined speed.

1. Download `chrome-merge-windows-x64.exe` from the **windows-latest** release,
   put it in its own folder, run it.
2. It checks which networks have internet, then opens a merged Chrome (its own
   window and logins). Everything else on the PC is untouched.

This is the PC cousin of the Android net-merge app. The same limit applies: **one
single connection still uses one network**, so one plain download stream or one
video does not get faster — but many connections at once do. For big single files
use a download manager set to several parts.

## netmerge-split — the flexible one

Assign **several apps to several networks** from a small config file, e.g. Chrome
on the cable and Brave on Wi-Fi, each with its own IP.

1. Download `netmerge-split-windows-x64.exe`, put it in its own folder, run it.
2. The first run writes `netmerge-split.json` next to it (detecting your networks
   and browsers) and starts them. Edit the file to taste and run again.

```json
{
  "links": [
    { "name": "cable", "interface": "Ethernet", "ip": "192.168.1.50", "port": 1080 },
    { "name": "wifi",  "interface": "Wi-Fi",    "ip": "192.168.1.77", "port": 1081 }
  ],
  "apps": [
    { "name": "Chrome", "path": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe", "link": "cable", "chromium": true },
    { "name": "Brave",  "path": "C:\\Program Files\\BraveSoftware\\Brave-Browser\\Application\\brave.exe", "link": "wifi", "chromium": true }
  ]
}
```

- **links** — one per network. `interface` is the adapter name from `ipconfig` /
  Network Connections; `ip` is a fallback if the name doesn't match. `port` is any
  free local port.
- **apps** — `link` is which link's `name` to use. `chromium: true` means the app
  takes `--proxy-server` (Chrome, Brave, Edge). Each Chromium app launches with its
  own profile folder so a fresh window opens and really uses the proxy, and so each
  keeps its own logins.

## How it works

For each network ("link") it starts a tiny local SOCKS5 proxy pinned to that
network card with the Windows `IP_UNICAST_IF` socket option, so its traffic really
leaves on that card even when another has a lower-metric route. DNS for the link
goes out the same card (1.1.1.1 / 8.8.8.8). Each browser is launched pointed at its
link's proxy.

## When it helps

Giving an app a different IP only works when your two networks use **different
internet lines** (e.g. cable to the router, Wi-Fi to a phone hotspot). If both go
to the same router, they already share one public IP and nothing changes.

"Everything else stays on the cable" assumes the cable is your default connection,
which it normally is when a cable is plugged in. If Wi-Fi is your default instead,
the rest of the PC follows Wi-Fi; set the cable as the preferred connection in
Windows, or ask and this can be handled in the tool.

## Limits

- One connection still uses one network — this doesn't speed up a single download.
  It assigns whole apps to networks.
- A browser already running may ignore a new window's proxy; the separate profile
  folder avoids this, but if in doubt close that browser fully first.
- Video calls / HTTP3 (QUIC, over UDP) fall back to normal TCP through the proxy.
- Firefox takes no proxy on the command line: in netmerge-split set `chromium:
  false` and point Firefox at SOCKS5 `127.0.0.1:<port>` by hand (tick "Proxy DNS
  when using SOCKS v5").

## Layout / build it yourself

```
windows/
  internal/split/   shared code: SOCKS proxy, interface pinning, config
  cmd/chrome-on-wifi/   the one-click Chrome-on-Wi-Fi program
  cmd/netmerge-split/   the flexible, config-driven program
```

```
cd windows
go test ./...
GOOS=windows GOARCH=amd64 go build -o chrome-on-wifi.exe ./cmd/chrome-on-wifi
GOOS=windows GOARCH=amd64 go build -o netmerge-split.exe ./cmd/netmerge-split
```

Pure Go standard library, no dependencies.
