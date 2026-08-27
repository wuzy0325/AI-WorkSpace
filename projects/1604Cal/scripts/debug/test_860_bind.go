//go:build ignore

package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	localAddrs := []string{"192.168.3.11", "192.168.1.11", ""}
	for _, local := range localAddrs {
		fmt.Printf("\n--- Trying local=%q -> 192.168.3.141:8000 ---\n", local)
		dialer := net.Dialer{Timeout: 5 * time.Second}
		if local != "" {
			localAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(local, "0"))
			if err != nil {
				fmt.Printf("  resolve local addr error: %v\n", err)
				continue
			}
			dialer.LocalAddr = localAddr
		}
		conn, err := dialer.Dial("tcp", "192.168.3.141:8000")
		if err != nil {
			fmt.Printf("  DIAL ERROR: %v\n", err)
			continue
		}
		defer conn.Close()
		fmt.Println("  CONNECTED OK")

		// Drain
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		drain := make([]byte, 4096)
		for { if _, err := conn.Read(drain); err != nil { break } }

		// *IDN?
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprintf(conn, "*IDN?\r\n")
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Printf("  *IDN? ERROR: %v\n", err)
		} else {
			fmt.Printf("  *IDN? RESPONSE: %q\n", string(buf[:n]))
		}
		conn.Close()
	}
}
