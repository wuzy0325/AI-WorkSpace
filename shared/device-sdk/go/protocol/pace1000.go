package protocol

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const PACE1000Query = ":sens?\r"

var ErrPACE1000InvalidResponse = errors.New("pace1000: invalid response")

// ParsePACE1000Pressure follows the LabVIEW %s%f scan contract. The first
// token is a device string and the second token is the pressure source value.
func ParsePACE1000Pressure(response []byte) (float64, error) {
	fields := strings.Fields(string(response))
	if len(fields) != 2 {
		return 0, fmt.Errorf("%w: expected string and float", ErrPACE1000InvalidResponse)
	}
	raw, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 0, fmt.Errorf("%w: invalid float", ErrPACE1000InvalidResponse)
	}
	pressurePa := raw * 1000
	if math.IsNaN(pressurePa) || math.IsInf(pressurePa, 0) {
		return 0, fmt.Errorf("%w: invalid pressure", ErrPACE1000InvalidResponse)
	}
	return pressurePa, nil
}
