// Package discovery 实现 KNXnet/IP 网关自动探测。
//
// 原理：向 KNX 标准组播地址 224.0.23.12:3671 发送 SEARCH_REQUEST
// （cEMI 报文头 0x06 0x10 0x02 0x01），符合规范的 KNX-IP 网关（含泰创
// TCP01RM）会单播回应 SEARCH_RESPONSE（0x02 0x02），其中携带
// control endpoint（网关的 IP:port）与设备信息 DIB。
//
// 兼容性兜底：若组播无响应（个别网关/路由器禁组播），可再发一次
// UDP 广播 255.255.255.255:3671。
package discovery

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	// MulticastGroup KNXnet/IP 标准组播地址
	MulticastGroup = "224.0.23.12"
	// BroadcastAddr 广播兜底地址
	BroadcastAddr = "255.255.255.255"
	// KNXPort KNXnet/IP 默认端口
	KNXPort = 3671

	headerSize = 6 // 0x06 0x10 0x02 0xXX + 2 字节长度
)

// Gateway 探测到的 KNX-IP 网关
type Gateway struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
	Name string `json:"name"` // 从设备信息 DIB 解析，可能为空
	Addr string `json:"addr"` // ip:port
}

// Options 探测参数
type Options struct {
	MulticastGroup    string
	Port              int
	Timeout           time.Duration
	BroadcastFallback bool
}

// DefaultOptions 返回默认探测参数
func DefaultOptions() Options {
	return Options{
		MulticastGroup:    MulticastGroup,
		Port:              KNXPort,
		Timeout:           5 * time.Second,
		BroadcastFallback: true,
	}
}

// BuildSearchRequest 构造 SEARCH_REQUEST 报文（含本地 HPAI）。
// 本地 IP 填 0.0.0.0 时大部分网关仍会响应；可传入本机出口 IP 提升兼容性。
func BuildSearchRequest(localIP net.IP) []byte {
	req := []byte{
		0x06, 0x10, 0x02, 0x01, // KNXnet/IP v1.0, SearchRequest
		0x00, 0x0e, // total length = 14
	}
	// HPAI (Host Protocol Address Information): 8 字节
	req = append(req, 0x08, 0x01) // code=UDP, len=8
	if v4 := localIP.To4(); v4 != nil {
		req = append(req, v4...)
	} else {
		req = append(req, 0, 0, 0, 0)
	}
	req = append(req, 0x00, 0x00) // 源端口 0（由系统分配）
	return req
}

// isSearchResponse 判断报文是否为 SEARCH_RESPONSE
func isSearchResponse(buf []byte) bool {
	if len(buf) < 10 {
		return false
	}
	return buf[0] == 0x06 && buf[1] == 0x10 && buf[2] == 0x02 && buf[3] == 0x02
}

// parseSearchResponse 解析 SEARCH_RESPONSE 中的 control endpoint 与设备名。
func parseSearchResponse(buf []byte) (ip string, port int, name string, err error) {
	if !isSearchResponse(buf) {
		return "", 0, "", fmt.Errorf("not a search response")
	}
	off := headerSize
	// 第一个 HPAI = control endpoint
	if len(buf) < off+8 {
		return "", 0, "", fmt.Errorf("truncated search response")
	}
	if buf[off] != 0x08 || buf[off+1] != 0x01 {
		return "", 0, "", fmt.Errorf("unexpected HPAI structure")
	}
	ip = net.IP(buf[off+2 : off+6]).String()
	port = int(binary.BigEndian.Uint16(buf[off+6 : off+8]))
	off += 8

	// 剩余为 DIB 块（device info 等）
	for off < len(buf) {
		dibLen := int(buf[off])
		if dibLen < 2 || off+dibLen > len(buf) {
			break
		}
		dibType := buf[off+1]
		if dibType == 0x01 { // Device Info DIB
			// KNXnet/IP Device Info DIB 的 friendly name 从第 24 字节开始。
			const nameOff = 24
			if dibLen > nameOff {
				raw := buf[off+nameOff : off+dibLen]
				// 名字可能以 0x00 结尾
				if i := strings.IndexByte(string(raw), 0); i >= 0 {
					raw = raw[:i]
				}
				name = strings.TrimSpace(string(raw))
			}
			break
		}
		off += dibLen
	}
	return ip, port, name, nil
}

// Discover 在局域网上探测全部 KNX-IP 网关。
// 返回按响应到达顺序的网关列表（去重）。
func Discover(opts Options) ([]Gateway, error) {
	if opts.Port == 0 {
		opts.Port = KNXPort
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.MulticastGroup == "" {
		opts.MulticastGroup = MulticastGroup
	}

	localIP := localOutboundIP()
	req := BuildSearchRequest(localIP)

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()

	// 设置组播出口接口（默认路由接口），部分系统必需
	if err := setMulticastInterface(conn); err != nil {
		return nil, fmt.Errorf("set multicast interface: %w", err)
	}

	dst := &net.UDPAddr{IP: net.ParseIP(opts.MulticastGroup), Port: opts.Port}
	if _, err := conn.WriteToUDP(req, dst); err != nil {
		return nil, fmt.Errorf("send search request (multicast): %w", err)
	}

	if opts.BroadcastFallback {
		bc := &net.UDPAddr{IP: net.ParseIP(BroadcastAddr), Port: opts.Port}
		if _, err := conn.WriteToUDP(req, bc); err != nil {
			// 广播失败不致命（可能被系统限制）
			_ = err
		}
	}

	deadline := time.Now().Add(opts.Timeout)
	conn.SetReadDeadline(deadline)

	seen := make(map[string]bool)
	var gateways []Gateway
	buf := make([]byte, 2048)
	for {
		n, source, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break // 探测窗口结束
			}
			break
		}
		ip, port, name, err := parseSearchResponse(buf[:n])
		if err != nil {
			continue
		}
		if ip == "0.0.0.0" && source != nil {
			ip = source.IP.String()
		}
		if port == 0 && source != nil {
			port = source.Port
		}
		addr := fmt.Sprintf("%s:%d", ip, port)
		if seen[addr] {
			continue
		}
		seen[addr] = true
		gateways = append(gateways, Gateway{IP: ip, Port: port, Name: name, Addr: addr})
	}
	return gateways, nil
}
