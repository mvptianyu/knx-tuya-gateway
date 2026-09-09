//go:build !windows

package discovery

import (
	"net"
	"syscall"
)

// setMulticastInterface 将组播报文绑定到默认出口接口。
// 通过 UDP 连接一个外部地址获取出口 IP，再设置 IP_MULTICAST_IF。
func setMulticastInterface(conn *net.UDPConn) error {
	ip := localOutboundIP()
	if ip == nil {
		return nil // 无法获取出口 IP 时跳过（组播通常仍可工作）
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		// IP_MULTICAST_IF 需要本机接口地址，以 IPv4 字节序传入
		sockErr = syscall.SetsockoptInet4Addr(
			int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, [4]byte(ip.To4()),
		)
	})
	if err != nil {
		return err
	}
	return sockErr
}

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
