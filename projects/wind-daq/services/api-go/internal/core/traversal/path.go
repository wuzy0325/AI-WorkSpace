package traversal

import (
	"fmt"
	"math"
)

func InterpolateLinearPath(points []Point, step float64) ([]Point, error) {
	if step <= 0 {
		return nil, fmt.Errorf("step must be greater than zero")
	}
	if len(points) < 2 {
		return nil, fmt.Errorf("at least two points are required")
	}

	path := []Point{points[0]}
	for i := 0; i < len(points)-1; i++ {
		from := points[i]
		to := points[i+1]
		distance := distance(from, to)
		if distance == 0 {
			continue
		}
		segments := int(math.Ceil(distance / step))
		for segment := 1; segment <= segments; segment++ {
			ratio := float64(segment) / float64(segments)
			path = append(path, Point{
				X: from.X + (to.X-from.X)*ratio,
				Y: from.Y + (to.Y-from.Y)*ratio,
				Z: from.Z + (to.Z-from.Z)*ratio,
			})
		}
	}
	return path, nil
}

func distance(a Point, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	dz := b.Z - a.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func GenerateGridPath(cfg GridConfig) ([]Point, error) {
	if cfg.XStep <= 0 || cfg.YStep <= 0 {
		return nil, fmt.Errorf("step must be greater than zero")
	}
	if cfg.XStart > cfg.XEnd || cfg.YStart > cfg.YEnd {
		return nil, fmt.Errorf("start must be less than or equal to end")
	}
	var path []Point
	for x := cfg.XStart; x <= cfg.XEnd; x += cfg.XStep {
		for y := cfg.YStart; y <= cfg.YEnd; y += cfg.YStep {
			path = append(path, Point{X: x, Y: y, Z: cfg.ZStart})
		}
	}
	return path, nil
}
