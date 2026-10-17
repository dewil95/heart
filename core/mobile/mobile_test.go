package mobile

import (
	"encoding/json"
	"testing"
)

func TestToNicsLeavesSourceToTheNetwork(t *testing.T) {
	ns := toNics([]network{
		{Name: "wlan0", Handle: 1, IP4: "192.168.1.5", V4: true},
		{Name: "rmnet_data0", Handle: 2, IP6: "2001:db8::5", V4: true, V6: true}, // 464XLAT
		{Name: "dead", Handle: 3},              // carries nothing
		{Name: "nohandle", V4: true, V6: true}, // can't be pinned
	})
	if len(ns) != 2 {
		t.Fatalf("got %d networks, want 2: %+v", len(ns), ns)
	}
	if !ns[0].IP.IsUnspecified() || ns[0].IP6 != nil {
		t.Fatalf("wlan0: %+v", ns[0])
	}
	if !ns[1].IP.IsUnspecified() || !ns[1].IP6.IsUnspecified() || ns[1].Handle != 2 {
		t.Fatalf("rmnet_data0: %+v", ns[1])
	}
}

func TestStatusWhenStopped(t *testing.T) {
	if err := SetNetworks(`[{"name":"wlan0","label":"Wi-Fi","handle":1,"v4":true}]`); err != nil {
		t.Fatal(err)
	}
	var st status
	if err := json.Unmarshal([]byte(Status()), &st); err != nil {
		t.Fatal(err)
	}
	if st.Running || st.Links == nil {
		t.Fatalf("status = %+v", st)
	}
	if SetNetworks("not json") == nil {
		t.Fatal("bad JSON should be an error")
	}
	if SetIPv6Mode("sometimes") == nil || SetIPv6Mode("off") != nil {
		t.Fatal("IPv6 mode parsing")
	}
}
