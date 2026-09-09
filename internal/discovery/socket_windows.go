//go:build windows

package discovery

import "net"

// setMulticastInterface Windows 上无需设置出口接口。
func setMulticastInterface(conn *net.UDPConn) error { return nil }

// localOutboundIP 通过 UDP 连接获取本机出口 IPv4（不真正发包）。
func localOutboundIP() net.IP {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.To4()
	}
	return nil
}
