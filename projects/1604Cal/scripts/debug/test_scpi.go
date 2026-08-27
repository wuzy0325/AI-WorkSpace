//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	conn, err := net.DialTimeout("tcp", "192.168.3.131:8000", 5*time.Second)
	if err != nil {
		fmt.Println("Dial error:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Connected")

	// Drain
	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	drainBuf := make([]byte, 4096)
	for {
		_, err := conn.Read(drainBuf)
		if err != nil {
			fmt.Println("Drain done:", err)
			break
		}
	}

	// Write
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = fmt.Fprintf(conn, "%s\r\n", "PRESsure:MODE VENT")
	if err != nil {
		fmt.Println("Write error:", err)
		return
	}
	fmt.Println("Written")

	// Read
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 4096)
	start := time.Now()
	n, err := conn.Read(buf)
	elapsed := time.Since(start)
	fmt.Printf("Read n=%d err=%v elapsed=%v data=%q\n", n, err, elapsed, string(buf[:n]))
}
