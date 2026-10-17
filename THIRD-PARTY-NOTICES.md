# Third-party notices

net-merge-android is MIT licensed (see `LICENSE`). It contains and links the following, all under
permissive licenses (license texts are in each project's repository and in the Go module cache):

| Component | License | Use |
|---|---|---|
| [net-merge](https://github.com/rohit267/net-merge) by Rohit Mahto | MIT | `core/engine` (scheduler, balancer, probes, SOCKS5) and `core/tunfd/forward.go` are ported from it, see `docs/PORTING.md` |
| [tun2socks](https://github.com/xjasonlyu/tun2socks) | MIT | TUN device + userland TCP/IP stack glue |
| [gVisor](https://github.com/google/gvisor) (netstack) | Apache-2.0 | userland TCP/IP stack |
| [golang.org/x/mobile](https://pkg.go.dev/golang.org/x/mobile) | BSD-3-Clause | gomobile bind: the Go library inside the app |
| golang.org/x/sys, golang.org/x/time and other golang.org/x modules | BSD-3-Clause | support code |
| Other transitive modules (see `core/go.mod` / `core/go.sum`) | MIT / Apache-2.0 / BSD | |
