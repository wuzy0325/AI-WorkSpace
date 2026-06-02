package core

// PtrFloat64 returns a pointer to a copy of v.
func PtrFloat64(v float64) *float64 { return &v }

// PtrInt returns a pointer to a copy of v.
func PtrInt(v int) *int { return &v }
