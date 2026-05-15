package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
)

// -- DAQ-P-1604 frame parsing --

// FrameReader implements 2-byte big-endian length-prefix framing
type FrameReader struct {
	conn net.Conn
}

func NewFrameReader(conn net.Conn) *FrameReader {
	return &FrameReader{conn: conn}
}

// ReadFrame reads one frame (2-byte big-endian length prefix + payload)
func (r *FrameReader) ReadFrame() ([]byte, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(r.conn, lenBuf[:]); err != nil {
		return nil, err
	}
	frameLen := int(binary.BigEndian.Uint16(lenBuf[:])) - 2
	if frameLen <= 0 {
		return nil, fmt.Errorf("invalid frame length: %d", frameLen+2)
	}

	payload := make([]byte, frameLen)
	if _, err := io.ReadFull(r.conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// IsASCIIFrame checks if data is ASCII (first 64 bytes in 0x20-0x7E + \r\n)
func IsASCIIFrame(data []byte) bool {
	checkLen := len(data)
	if checkLen > 64 {
		checkLen = 64
	}
	for _, b := range data[:checkLen] {
		if b != 0x0d && b != 0x0a && (b < 0x20 || b > 0x7e) {
			return false
		}
	}
	return true
}

// ParseStreamFrame parses binary stream frame
// Frame: 5-byte header + 18 x float32 (big-endian) = 77 bytes
// Device order: CH16..CH1 (pressure) + CH17 (atm pressure) + CH18 (atm temp)
// Reverses first 16 pressure channels to CH1..CH16
func ParseStreamFrame(data []byte) ([]float64, error) {
	const headerSize = 5
	const numChannels = 18
	const numPressure = 16
	expectedLen := headerSize + numChannels*4

	if len(data) < expectedLen {
		return nil, fmt.Errorf("frame too short: %d, expected %d", len(data), expectedLen)
	}

	channels := make([]float64, numChannels)
	for i := 0; i < numChannels; i++ {
		bits := binary.BigEndian.Uint32(data[headerSize+i*4:])
		channels[i] = float64(math.Float32frombits(bits))
	}

	for i := 0; i < numPressure/2; i++ {
		channels[i], channels[numPressure-1-i] = channels[numPressure-1-i], channels[i]
	}

	return channels, nil
}
