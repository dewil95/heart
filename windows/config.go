package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Config is the netmerge-split.json file sitting next to the program. A "link"
// is a network to send through; an "app" is a program pinned to one link.
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

func loadConfig(path string) (*Config, error) {
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

func saveConfig(path string, c *Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (c *Config) linkConfig(name string) *LinkConfig {
	for i := range c.Links {
		if c.Links[i].Name == name {
			return &c.Links[i]
		}
	}
	return nil
}

// iface is one usable network adapter found on the machine.
type iface struct {
	name  string
	index int
	ip    net.IP
	wifi  bool
}

// usableInterfaces lists adapters that are up, not loopback and have a real
// IPv4 address, guessing from the name whether each is Wi-Fi.
func usableInterfaces() []iface {
	var out []iface
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
		out = append(out, iface{name: in.Name, index: in.Index, ip: ip, wifi: wifi})
	}
	return out
}

// generateConfig builds a starting config from what the machine has: one link
// per adapter, and any Chrome/Brave/Edge found, pinned to separate links so the
// common "one browser per network" setup works out of the box.
func generateConfig() *Config {
	c := &Config{}
	used := map[string]bool{}
	port := 1080
	for _, in := range usableInterfaces() {
		base := "cable"
		if in.wifi {
			base = "wifi"
		}
		name := base
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s%d", base, n)
		}
		used[name] = true
		c.Links = append(c.Links, LinkConfig{Name: name, Interface: in.name, IP: in.ip.String(), Port: port})
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
	for _, b := range knownBrowsers {
		path := findBrowser(b)
		if path == "" {
			continue
		}
		link := first
		if b.name == "Brave" { // put the second browser on the second link, if any
			link = second
		}
		c.Apps = append(c.Apps, AppConfig{Name: b.name, Path: path, Link: link, Chromium: b.chromium})
	}
	return c
}

// resolveLink turns a LinkConfig into a live Link by matching it to an adapter,
// first by interface name, then by IP address.
func resolveLink(lc LinkConfig, ifs []iface) (*Link, error) {
	var match *iface
	for i := range ifs {
		if lc.Interface != "" && strings.EqualFold(ifs[i].name, lc.Interface) {
			match = &ifs[i]
			break
		}
	}
	if match == nil && lc.IP != "" {
		for i := range ifs {
			if ifs[i].ip.String() == lc.IP {
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
		Interface:  match.name,
		IfaceIndex: match.index,
		SourceIP:   match.ip,
	}, nil
}
