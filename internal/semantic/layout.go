package semantic

import (
	"context"
	"math"
	"sort"
)

// Point is a post's place on the library map, each coordinate from 0 to 1.
type Point struct {
	ID int64   `json:"id"`
	X  float32 `json:"x"`
	Y  float32 `json:"y"`
}

// layout is the map worked out for one snapshot of the vectors.
type layout struct {
	from   *snapshot
	points []Point
}

// Layout places every indexed post on a flat map, so posts about the same
// thing sit near each other. The two axes are the directions in which the
// library's vectors differ most (its first two principal components). It
// is worked out from the vectors already on this computer, never asking
// the AI service, and kept until posts are indexed or deleted.
func (s *Service) Layout(ctx context.Context) ([]Point, error) {
	if !s.store.MeaningSearchEnabled(ctx) {
		return nil, ErrOff
	}
	vectors, err := s.fresh(ctx)
	if err != nil {
		return nil, err
	}
	if cached := s.mapped.Load(); cached != nil && cached.from == vectors {
		return cached.points, nil
	}
	s.mapping.Lock()
	defer s.mapping.Unlock()
	if cached := s.mapped.Load(); cached != nil && cached.from == vectors {
		return cached.points, nil
	}
	points := project(vectors)
	s.mapped.Store(&layout{from: vectors, points: points})
	return points, nil
}

// The most rounds of power iteration per axis. It starts from a fixed
// direction, so the same vectors always give the same map.
const layoutRounds = 60

// layoutSettled is how little an axis may still turn in a round for the
// iteration to stop early: well under the width of a dot on the map.
const layoutSettled = 1e-7

// project returns each post's place along the snapshot's first two principal
// components, scaled into 0 to 1.
func project(vectors *snapshot) []Point {
	n, size := len(vectors.ids), vectors.size
	if n == 0 {
		return []Point{}
	}
	// Each vector as a unit direction, less the library's average direction,
	// so the axes describe how posts differ rather than what they share.
	rows := make([][]float64, 0, n)
	mean := make([]float64, size)
	for index := range vectors.ids {
		row := make([]float64, size)
		if norm := float64(vectors.lengths[index]); norm > 0 {
			for position, value := range vectors.vector(index) {
				row[position] = float64(value) / norm
				mean[position] += row[position]
			}
		}
		rows = append(rows, row)
	}
	for position := range mean {
		mean[position] /= float64(n)
	}
	for _, row := range rows {
		for position := range row {
			row[position] -= mean[position]
		}
	}

	first := component(rows, size, nil)
	second := component(rows, size, first)
	xs, ys := make([]float64, n), make([]float64, n)
	for index, row := range rows {
		xs[index], ys[index] = dot(row, first), dot(row, second)
	}
	scaleX, scaleY := spread(xs), spread(ys)
	points := make([]Point, n)
	for index, id := range vectors.ids {
		points[index] = Point{ID: id, X: scaleX(xs[index]), Y: scaleY(ys[index])}
	}
	return points
}

// component finds the direction of greatest variance in rows, at right
// angles to after when given, by power iteration from a fixed start. Its
// sign is chosen so its largest entry is positive: a direction and its
// opposite describe the same axis, and this keeps the map from flipping.
func component(rows [][]float64, size int, after []float64) []float64 {
	direction := make([]float64, size)
	for position := range direction {
		direction[position] = 1 + float64(position%7)
	}
	toUnit(direction, after)
	for round := 0; round < layoutRounds; round++ {
		next := make([]float64, size)
		for _, row := range rows {
			weight := dot(row, direction)
			for position, value := range row {
				next[position] += weight * value
			}
		}
		if !toUnit(next, after) {
			break
		}
		// Settled: another round would barely turn it.
		settled := math.Abs(dot(next, direction)) > 1-layoutSettled
		direction = next
		if settled {
			break
		}
	}
	largest := 0
	for position, value := range direction {
		if math.Abs(value) > math.Abs(direction[largest]) {
			largest = position
		}
	}
	if direction[largest] < 0 {
		for position := range direction {
			direction[position] = -direction[position]
		}
	}
	return direction
}

// toUnit removes any part of vector along after, then scales it to
// length one. It reports false when nothing is left.
func toUnit(vector, after []float64) bool {
	if after != nil {
		along := dot(vector, after)
		for position := range vector {
			vector[position] -= along * after[position]
		}
	}
	length := math.Sqrt(dot(vector, vector))
	if length == 0 || math.IsNaN(length) {
		return false
	}
	for position := range vector {
		vector[position] /= length
	}
	return true
}

func dot(a, b []float64) float64 {
	var sum float64
	for position := range a {
		sum += a[position] * b[position]
	}
	return sum
}

// spread maps values into 0 to 1 by where they fall between the 2nd and
// 98th percentile, so a few far-off posts don't squeeze the rest into a
// corner; those few sit on the edge.
func spread(values []float64) func(float64) float32 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	low := sorted[len(sorted)*2/100]
	high := sorted[(len(sorted)-1)*98/100]
	return func(value float64) float32 {
		if high <= low {
			return 0.5
		}
		scaled := (value - low) / (high - low)
		return float32(math.Round(math.Min(1, math.Max(0, scaled))*1000) / 1000)
	}
}
