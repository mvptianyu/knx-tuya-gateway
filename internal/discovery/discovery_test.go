package discovery

import (
	"net"
	"testing"
)

func TestBuildSearchRequest(t *testing.T) {
	req := BuildSearchRequest(net.IPv4(192, 168, 1, 10))
	want := []byte{
		0x06, 0x10, 0x02, 0x01, // SearchRequest
		0x00, 0x0E, // total length
		0x08, 0x01, // HPAI UDP
		192, 168, 1, 10,
		0x00, 0x00, // source port
	}
	if len(req) != len(want) {
		t.Fatalf("len = %d, want %d", len(req), len(want))
	}
	for i := range want {
		if req[i] != want[i] {
			t.Fatalf("byte %d = 0x%02x, want 0x%02x", i, req[i], want[i])
		}
	}
}

func TestParseSearchResponse(t *testing.T) {
	// 构造 SEARCH_RESPONSE：
	// header 06 10 02 02 + len(2)
	// HPAI: 08 01 IP(C0 A8 6E E8=192.168.110.232) port(0E 57=3671)
	// DIB device info: standard fields followed by a 30-byte friendly name.
	buf := []byte{
		0x06, 0x10, 0x02, 0x02, 0x00, 0x44,
		0x08, 0x01, 0xC0, 0xA8, 0x6E, 0xE8, 0x0E, 0x57,
		0x36, 0x01, // DIB len=54, type=device info
		0x02, 0x00, // medium, status
		0x11, 0x01, // individual address
		0x00, 0x01, // project installation id
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, // serial
		0xE0, 0x00, 0x17, 0x0C, // multicast
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, // MAC
		'T', 'P', '-', 'D', 'E', 'V', 'I', 'C', 'E', 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	ip, port, name, err := parseSearchResponse(buf)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ip != "192.168.110.232" {
		t.Errorf("ip = %s, want 192.168.110.232", ip)
	}
	if port != 3671 {
		t.Errorf("port = %d, want 3671", port)
	}
	if name != "TP-DEVICE" {
		t.Errorf("name = %q, want TP-DEVICE", name)
	}
}

func TestIsSearchResponse(t *testing.T) {
	if !isSearchResponse([]byte{0x06, 0x10, 0x02, 0x02, 0x00, 0x24, 0, 0, 0, 0}) {
		t.Error("should be search response")
	}
	if isSearchResponse([]byte{0x06, 0x10, 0x02, 0x01, 0x00, 0x08}) {
		t.Error("search request should not match")
	}
}
