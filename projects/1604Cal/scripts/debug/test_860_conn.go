//go:build ignore

package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	addr := "192.168.3.141:8000"
	fmt.Printf("Dialing %s ...\n", addr)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		fmt.Printf("DIAL ERROR: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Println("CONNECTED")

	// Drain any banner
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	drain := make([]byte, 4096)
	for {
		_, err := conn.Read(drain)
		if err != nil {
			break
		}
	}

	// Send *IDN?
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = fmt.Fprintf(conn, "*IDN?\r\n")
	if err != nil {
		fmt.Printf("WRITE ERROR: %v\n", err)
		return
	}
	fmt.Println("Sent *IDN?")

	// Read response
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("READ ERROR: %v (after %v)\n", err, 5*time.Second)
	} else {
		fmt.Printf("RESPONSE: %q\n", string(buf[:n]))
	}

	// Try PRESsure?
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = fmt.Fprintf(conn, "PRESsure?\r\n")
	if err != nil {
		fmt.Printf("WRITE ERROR2: %v\n", err)
		return
	}
	fmt.Println("Sent PRESsure?")

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err = conn.Read(buf)
	if err != nil {
		fmt.Printf("READ ERROR2: %v\n", err)
	} else {
		fmt.Printf("PRESSURE: %q\n", string(buf[:n]))
	}
}
