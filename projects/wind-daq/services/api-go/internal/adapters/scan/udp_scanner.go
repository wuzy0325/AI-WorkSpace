package scan

import (
	"encoding/binary"
	"log/slog"
	"net"
)

// ipv4ToInt 将 IPv4 地址转换为 uint32
func ipv4ToInt(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

// intToIPv4 将 uint32 转换为 IPv4 地址
func intToIPv4(v uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, v)
	return ip
}

// computeBroadcastAddress 根据地址和子网掩码计算广播地址
func computeBroadcastAddress(address net.IP, mask net.IPMask) net.IP {
	addrInt := ipv4ToInt(address)
	maskInt := ipv4ToInt(net.IP(mask))
	broadcastInt := (addrInt | ^maskInt)
	return intToIPv4(broadcastInt)
}

// BroadcastTarget 广播目标
type BroadcastTarget struct {
	LocalAddress string
	BroadcastIP  string
}

// getBroadcastTargets 自动枚举网络接口，计算广播地址
// 排除内部接口（loopback），只使用 IPv4
func getBroadcastTargets() []BroadcastTarget {
	var targets []BroadcastTarget

	interfaces, err := net.Interfaces()
	if err != nil {
		slog.Error("Failed to enumerate network interfaces", "err", err)
		return targets
	}

	seen := make(map[string]bool)

	for _, iface := range interfaces {
		// 跳过 loopback 和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue // 跳过 IPv6
			}

			broadcastIP := computeBroadcastAddress(ip, ipNet.Mask)
			broadcastStr := broadcastIP.String()

			if !seen[broadcastStr] {
				seen[broadcastStr] = true
				targets = append(targets, BroadcastTarget{
					LocalAddress: ip.String(),
					BroadcastIP:  broadcastStr,
				})
			}
		}
	}

	return targets
}
