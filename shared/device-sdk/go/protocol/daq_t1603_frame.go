package protocol

import (
	"encoding/binary"
	"fmt"
	"math"
)

// -- DAQ-T-1603 serial frame parsing --

const SerialFrameSize = 46

// ParseSerialFrame parses serial data frame
// Temperature data: 16 channels x 2 bytes, signed short, big-endian, 0.1 C/LSB
func ParseSerialFrame(data []byte) ([]float64, error) {
	if len(data) != SerialFrameSize {
		return nil, fmt.Errorf("invalid frame size: %d, expected %d", len(data), SerialFrameSize)
	}

	temperatures := make([]float64, 16)
	for i := 0; i < 16; i++ {
		raw := int16(binary.BigEndian.Uint16(data[8+i*2:]))
		temperatures[i] = float64(raw) * 0.1
	}
	return temperatures, nil
}

// -- DAQ-T-1603 TCP frame parsing --

const TCPFrameSize = 64

// ParseTCPFrame parses TCP data frame
// Channel order: buffer[0] is channel 15, buffer[15] is channel 0, needs reversal
func ParseTCPFrame(data []byte) ([]float64, error) {
	if len(data) != TCPFrameSize {
		return nil, fmt.Errorf("invalid frame size: %d, expected %d", len(data), TCPFrameSize)
	}

	temperatures := make([]float64, 16)
	for i := 0; i < 16; i++ {
		bits := binary.LittleEndian.Uint32(data[i*4:])
		temperatures[i] = float64(math.Float32frombits(bits))
	}

	for i, j := 0, len(temperatures)-1; i < j; i, j = i+1, j-1 {
		temperatures[i], temperatures[j] = temperatures[j], temperatures[i]
	}

	return temperatures, nil
}
