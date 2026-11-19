package split

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Config is the netmerge-split.json file used by the flexible launcher. A
// "link" is a network to send through; an "app" is a program pinned to one link.
type Config struct {
	Links []LinkConfig `json:"links"`
	Apps  []AppConfig  `json:"apps"`
}

type LinkConfig struct {
	Name      string `json:"name"`                // friendly name, e.g. "cable"
	Interface string `json:"interface,omitempty"` // Windows adapter name, e.g. "Ethernet"
	IP        string `json:"ip,omitempty"`        // the adapter's IPv4, as a fallback match
	Port      int    `json:"port"`                // local SOCKS port for this link
}

type AppConfig struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Link     string   `json:"link"`           // which link's name this app uses
	Chromium bool     `json:"chromium"`       // Chrome/Brave/Edge take --proxy-server
	Args     []string `json:"args,omitempty"` // extra command-line arguments
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s is not valid: %w", path, err)
	}
	return &c, nil
}

func SaveConfig(path string, c *Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (c *Config) LinkConfig(name string) *LinkConfig {
	for i := range c.Links {
		if c.Links[i].Name == name {
			return &c.Links[i]
		}
	}
	return nil
}

// Iface is one usable network adapter found on the machine.
type Iface struct {
	Name  string
	Index int
	IP    net.IP
	Wifi  bool
}

// UsableInterfaces lists adapters that are up, not loopback and have a real
// IPv4 address, guessing from the name whether each is Wi-Fi.
func UsableInterfaces() []Iface {
	var out []Iface
	ifs, _ := net.Interfaces()
	for _, in := range ifs {
		if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 {
			continue
		}
		var ip net.IP
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if v4 := n.IP.To4(); v4 != nil && !v4.IsLinkLocalUnicast() && !v4.IsLoopback() {
				ip = v4
				break
			}
		}
		if ip == nil {
			continue
		}
		low := strings.ToLower(in.Name)
		wifi := strings.Contains(low, "wi-fi") || strings.Contains(low, "wifi") ||
			strings.Contains(low, "wireless") || strings.Contains(low, "wlan")
		out = append(out, Iface{Name: in.Name, Index: in.Index, IP: ip, Wifi: wifi})
	}
	return out
}

// WifiInterface returns the first usable Wi-Fi adapter.
func WifiInterface() (Iface, bool) {
	for _, in := range UsableInterfaces() {
		if in.Wifi {
			return in, true
		}
	}
	return Iface{}, false
}

// GenerateConfig builds a starting config from what the machine has: one link
// per adapter, and any Chrome/Brave/Edge found, pinned to separate links so the
// common "one browser per network" setup works out of the box.
func GenerateConfig() *Config {
	c := &Config{}
	used := map[string]bool{}
	port := 1080
	for _, in := range UsableInterfaces() {
		base := "cable"
		if in.Wifi {
			base = "wifi"
		}
		name := base
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s%d", base, n)
		}
		used[name] = true
		c.Links = append(c.Links, LinkConfig{Name: name, Interface: in.Name, IP: in.IP.String(), Port: port})
		port++
	}

	first, second := "", ""
	if len(c.Links) > 0 {
		first = c.Links[0].Name
		second = first
	}
	if len(c.Links) > 1 {
		second = c.Links[1].Name
	}
	for _, b := range KnownBrowsers {
		path := FindBrowser(b)
		if path == "" {
			continue
		}
		link := first
		if b.Name == "Brave" { // put the second browser on the second link, if any
			link = second
		}
		c.Apps = append(c.Apps, AppConfig{Name: b.Name, Path: path, Link: link, Chromium: b.Chromium})
	}
	return c
}

// ResolveLink turns a LinkConfig into a live Link by matching it to an adapter,
// first by interface name, then by IP address.
func ResolveLink(lc LinkConfig, ifs []Iface) (*Link, error) {
	var match *Iface
	for i := range ifs {
		if lc.Interface != "" && strings.EqualFold(ifs[i].Name, lc.Interface) {
			match = &ifs[i]
			break
		}
	}
	if match == nil && lc.IP != "" {
		for i := range ifs {
			if ifs[i].IP.String() == lc.IP {
				match = &ifs[i]
				break
			}
		}
	}
	if match == nil {
		return nil, fmt.Errorf("no live adapter matches interface %q / ip %q", lc.Interface, lc.IP)
	}
	return &Link{
		Name:       lc.Name,
		Interface:  match.Name,
		IfaceIndex: match.Index,
		SourceIP:   match.IP,
	}, nil
}
