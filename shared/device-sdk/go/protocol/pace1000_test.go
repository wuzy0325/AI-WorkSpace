package protocol

import (
	"math"
	"testing"
)

func TestParsePACE1000PressureDiscardsStringAndConvertsFloat(t *testing.T) {
	got, err := ParsePACE1000Pressure([]byte("PACE 101.325\r\n"))
	if err != nil {
		t.Fatalf("ParsePACE1000Pressure returned error: %v", err)
	}
	if got != 101325 {
		t.Fatalf("expected 101325 Pa, got %v", got)
	}
}

func TestParsePACE1000PressureRejectsWrongFieldCount(t *testing.T) {
	for _, response := range []string{"", "101.325", "PACE", "PACE 101.325 extra"} {
		if _, err := ParsePACE1000Pressure([]byte(response)); err == nil {
			t.Errorf("expected %q to be rejected", response)
		}
	}
}

func TestParsePACE1000PressureRejectsNonFiniteOrInvalidFloat(t *testing.T) {
	for _, response := range []string{"PACE nope", "PACE NaN", "PACE +Inf", "PACE -Inf"} {
		if _, err := ParsePACE1000Pressure([]byte(response)); err == nil {
			t.Errorf("expected %q to be rejected", response)
		}
	}
	if math.IsNaN(101325) {
		t.Fatal("test sanity check failed")
	}
}

func TestPACE1000QueryUsesCRWithoutLF(t *testing.T) {
	if PACE1000Query != ":sens?\r" {
		t.Fatalf("expected CR-terminated query, got %q", PACE1000Query)
	}
}
