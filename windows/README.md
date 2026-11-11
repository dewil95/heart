# net-merge per-app for Windows

Send **each app out a different network**. For example: Chrome over the Ethernet
cable, Brave over Wi-Fi — each browser then has its own IP address.

This is the opposite of the Android app. The Android app *merges* one app's
traffic across both networks for speed. This tool *separates* apps onto
different networks. (Windows has no clean, driver-free way to merge one app
across links; separating is the part that works reliably.)

## What it's good for

- Two browsers signed in to the same website as two different connections
  (different IP each), e.g. two accounts at once.
- Keeping one app on a cheap/unmetered link and another on a metered one.
- Sending a specific app out a specific internet line when you have two.

It only helps when your two networks use **different internet lines** (e.g.
cable to your router, Wi-Fi to a phone hotspot). If both go to the same router,
both already share one public IP and nothing changes.

## How it works

For every network ("link") it starts a tiny local proxy. Each proxy is pinned to
one network card with the Windows `IP_UNICAST_IF` socket option, so its traffic
really leaves on that card even if the other has a "better" route. DNS for that
link goes out the same card (1.1.1.1 / 8.8.8.8). Each browser is launched pointed
at its link's proxy, so all of that browser's traffic uses the chosen network.

No driver. No admin rights. No DLL injection. One `.exe`.

## Use it

1. Download `netmerge-split-windows-x64.exe` from the **windows-latest** release
   (use the arm64 file on a Snapdragon/ARM laptop). Put it in its own folder,
   e.g. `C:\netmerge`.
2. Double-click it. The first run detects your networks and browsers and writes
   `netmerge-split.json` next to the program, then starts your browsers — each on
   its own network. Keep the black window open; closing it stops the proxies.
3. Check it worked: open a "what is my IP" page in each browser. If your two
   networks use different internet lines, the two browsers show different IPs.

Windows SmartScreen may warn the first time because the file isn't signed:
**More info → Run anyway**.

## Change which app uses which network

Edit `netmerge-split.json` (plain text) and run the program again.

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

- **links** — one per network. `interface` is the adapter name shown in
  `ipconfig` / Network Connections; `ip` is a fallback if the name doesn't match.
  `port` is any free local port.
- **apps** — `link` is which link's `name` to use. `chromium: true` means the app
  takes `--proxy-server` (Chrome, Brave, Edge, Vivaldi, Opera). Each Chromium app
  is launched with its own profile folder (under `profiles\`) so a fresh window
  opens and really uses the proxy, and so each keeps its own logins.

## Other apps / Firefox

Set `chromium: false` and the tool will start the app but can't set its proxy for
it. Point the app at the SOCKS proxy by hand: Firefox → Settings → Network →
Manual proxy → SOCKS v5 host `127.0.0.1`, port as in the config, and tick "Proxy
DNS when using SOCKS v5".

## Limits

- One connection still uses one network — this doesn't speed up a single
  download. It assigns whole apps to networks.
- A browser already running may ignore a new window's proxy; the separate profile
  folder avoids this, but if in doubt close the browser fully first.
- Video calls/HTTP3 (QUIC, over UDP) fall back to normal TCP through the proxy.

## Build it yourself

```
cd windows
go test ./...
GOOS=windows GOARCH=amd64 go build -o netmerge-split.exe .
```

Pure Go standard library, no dependencies.
