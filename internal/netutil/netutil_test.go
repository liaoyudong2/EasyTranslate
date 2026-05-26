package netutil

import (
	"net"
	"testing"
)

func TestUsableIPv4(t *testing.T) {
	_, cidr, err := net.ParseCIDR("192.168.215.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if usableIPv4(net.ParseIP("192.168.215.0").To4(), cidr) {
		t.Fatal("network address should not be usable")
	}
	if usableIPv4(net.ParseIP("192.168.215.255").To4(), cidr) {
		t.Fatal("broadcast address should not be usable")
	}
	if !usableIPv4(net.ParseIP("192.168.215.10").To4(), cidr) {
		t.Fatal("private host address should be usable")
	}
}

func TestUsableInterface(t *testing.T) {
	if usableInterface(net.Interface{Name: "bridge100", Flags: net.FlagUp}) {
		t.Fatal("bridge interface should be filtered")
	}
	if usableInterface(net.Interface{Name: "utun0", Flags: net.FlagUp | net.FlagPointToPoint}) {
		t.Fatal("point-to-point interface should be filtered")
	}
	if !usableInterface(net.Interface{Name: "en7", Flags: net.FlagUp | net.FlagBroadcast}) {
		t.Fatal("active physical interface should be usable")
	}
}
