//go:build !windows

package scan

import (
	"fmt"
	"net"
)

func openDiscoverySocket(localPort int) (discoverySocket, error) {
	conn, err := net.ListenPacket("udp4", net.JoinHostPort("", fmt.Sprint(localPort)))
	if err != nil {
		return nil, err
	}
	return &packetDiscoverySocket{conn: conn}, nil
}
