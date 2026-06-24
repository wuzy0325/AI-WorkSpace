package httpapi

import "shared.local/device-sdk/go/motion/core"

// HTTP 出参只使用 map/slice/原生值，绕开当前 Go 版本 encoding/json 的 struct 反射崩溃。
func toStatusDTOs(list []core.ControllerStatus) []map[string]any {
	if len(list) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, len(list))
	for i, s := range list {
		axes := make([]map[string]any, len(s.Axes))
		for j, a := range s.Axes {
			axis := map[string]any{
				"name":          string(a.Name),
				"position":      a.Position,
				"velocity":      a.Velocity,
				"moving":        a.Moving,
				"homed":         a.Homed,
				"posLimit":      a.PosLimit,
				"negLimit":      a.NegLimit,
				"compensating":  a.Compensating,
				"positionError": a.PositionError,
			}
			if a.CompensationError != "" {
				axis["compensationError"] = a.CompensationError
			}
			axes[j] = axis
		}

		status := map[string]any{
			"id":               s.ID,
			"name":             s.Name,
			"type":             string(s.Type),
			"connected":        s.Connected,
			"emergencyStopped": s.EmergencyStopped,
			"axes":             axes,
		}
		if s.LastError != "" {
			status["lastError"] = s.LastError
		}
		out[i] = status
	}
	return out
}

func toProfileDTOs(list []core.MotionControllerProfile) []map[string]any {
	if len(list) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, len(list))
	for i, p := range list {
		axes := make([]map[string]any, len(p.Axes))
		for j, a := range p.Axes {
			axis := map[string]any{
				"name":            string(a.Name),
				"enabled":         a.Enabled,
				"kind":            string(a.Kind),
				"inverted":        a.Inverted,
				"encoderInverted": a.EncoderInverted,
				"positionSource":  string(a.PositionSource),
			}
			setFloat(axis, "stepsPerRev", a.StepsPerRev)
			setInt(axis, "microSteps", a.MicroSteps)
			setFloat(axis, "lead", a.Lead)
			setFloat(axis, "gearRatio", a.GearRatio)
			setFloat(axis, "maxSpeed", a.MaxSpeed)
			setFloat(axis, "minLimit", a.MinLimit)
			setFloat(axis, "maxLimit", a.MaxLimit)
			setFloat(axis, "encoderScale", a.EncoderScale)
			if a.EncoderCompensation != nil {
				axis["encoderCompensation"] = map[string]any{
					"enabled":   a.EncoderCompensation.Enabled,
					"tolerance": a.EncoderCompensation.Tolerance,
					"maxCycles": a.EncoderCompensation.MaxCycles,
					"settleMs":  a.EncoderCompensation.SettleMs,
					"minStep":   a.EncoderCompensation.MinStep,
					"timeoutMs": a.EncoderCompensation.TimeoutMs,
				}
			}
			axes[j] = axis
		}

		out[i] = map[string]any{
			"id":          p.ID,
			"name":        p.Name,
			"type":        string(p.Type),
			"address":     p.Address,
			"port":        p.Port,
			"autoConnect": p.AutoConnect,
			"axes":        axes,
		}
	}
	return out
}

func setFloat(target map[string]any, key string, value *float64) {
	if value != nil {
		target[key] = *value
	}
}

func setInt(target map[string]any, key string, value *int) {
	if value != nil {
		target[key] = *value
	}
}
