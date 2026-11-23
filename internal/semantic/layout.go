package semantic

import (
	"context"
	"hash/fnv"
	"math"
	"sort"
)

// Point is a post's place on the library map. X runs from 0 to 1 and Y from
// 0 to the map's aspect (its height over its width), so distances mean the
// same across and down.
type Point struct {
	ID int64   `json:"id"`
	X  float32 `json:"x"`
	Y  float32 `json:"y"`
}

// Island is one group's place on the map: its centre and radius, in the same
// units as the points.
type Island struct {
	Group  int64   `json:"category"`
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Radius float32 `json:"radius"`
}

// Map is the library drawn as an archipelago: one island per group, islands
// about related things near each other, and within an island, posts about
// the same thing together.
type Map struct {
	Points  []Point  `json:"points"`
	Islands []Island `json:"islands"`
	// Aspect is the map's height over its width.
	Aspect float32 `json:"aspect"`
}

// drawn is the map worked out for one snapshot and one grouping.
type drawn struct {
	from    *snapshot
	grouped uint64
	result  Map
}

// Layout draws the map for the posts in groups (post ID to its group, such
// as its main category; posts left out are left off the map). It is worked
// out from the vectors already on this computer, never asking the AI
// service, and kept until posts are indexed or regrouped.
func (s *Service) Layout(ctx context.Context, groups map[int64]int64) (Map, error) {
	if !s.store.MeaningSearchEnabled(ctx) {
		return Map{}, ErrOff
	}
	vectors, err := s.fresh(ctx)
	if err != nil {
		return Map{}, err
	}
	signature := groupingSignature(groups)
	if cached := s.mapped.Load(); cached != nil && cached.from == vectors && cached.grouped == signature {
		return cached.result, nil
	}
	s.mapping.Lock()
	defer s.mapping.Unlock()
	if cached := s.mapped.Load(); cached != nil && cached.from == vectors && cached.grouped == signature {
		return cached.result, nil
	}
	result := archipelago(vectors, groups)
	s.mapped.Store(&drawn{from: vectors, grouped: signature, result: result})
	return result, nil
}

// groupingSignature tells one grouping from another, in any order.
func groupingSignature(groups map[int64]int64) uint64 {
	ids := make([]int64, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	hash := fnv.New64a()
	buffer := make([]byte, 16)
	for _, id := range ids {
		for shift := 0; shift < 8; shift++ {
			buffer[shift] = byte(id >> (8 * shift))
			buffer[8+shift] = byte(groups[id] >> (8 * shift))
		}
		hash.Write(buffer)
	}
	return hash.Sum64()
}

// The most rounds of power iteration per axis. It starts from a fixed
// direction, so the same vectors always give the same map.
const layoutRounds = 60

// layoutSettled is how little an axis may still turn in a round for the
// iteration to stop early.
const layoutSettled = 1e-7

// Islands are laid out a little wider than tall, to suit a screen.
const layoutWidth = 1.7

// island is one group while the map is worked out.
type island struct {
	group   int64
	members []int     // rows of the snapshot
	centre  []float64 // the group's average direction
	x, y    float64   // its place on the map
	radius  float64
}

// archipelago lays the posts out. Each group becomes a round island whose
// area grows with its number of posts. Islands start where their average
// meanings fall along the two directions the groups differ most, then are
// pushed apart until none overlap, so related topics stay neighbours. Inside
// an island, posts are placed by how they differ from the island's average,
// then spread evenly over its area, keeping each one's direction from the
// centre, so similar posts stay together without piling up.
func archipelago(vectors *snapshot, groups map[int64]int64) Map {
	size := vectors.size
	byGroup := map[int64]*island{}
	var order []*island
	for index, id := range vectors.ids {
		group, ok := groups[id]
		if !ok || vectors.lengths[index] == 0 {
			continue
		}
		current := byGroup[group]
		if current == nil {
			current = &island{group: group, centre: make([]float64, size)}
			byGroup[group] = current
			order = append(order, current)
		}
		current.members = append(current.members, index)
		norm := float64(vectors.lengths[index])
		for position, value := range vectors.vector(index) {
			current.centre[position] += float64(value) / norm
		}
	}
	if len(order) == 0 {
		return Map{Points: []Point{}, Islands: []Island{}, Aspect: 1 / layoutWidth}
	}
	// Biggest first, then by group, so the result never depends on map order.
	sort.Slice(order, func(i, j int) bool {
		if len(order[i].members) != len(order[j].members) {
			return len(order[i].members) > len(order[j].members)
		}
		return order[i].group < order[j].group
	})
	for _, current := range order {
		for position := range current.centre {
			current.centre[position] /= float64(len(current.members))
		}
		current.radius = math.Sqrt(float64(len(current.members)))
	}

	placeIslands(order, size)

	points := make([]Point, 0, len(vectors.ids))
	type placed struct {
		id   int64
		x, y float64
	}
	var all []placed
	for _, current := range order {
		for _, spot := range spreadIsland(vectors, current) {
			all = append(all, placed{id: spot.id, x: spot.x, y: spot.y})
		}
	}

	// Fit everything, islands' edges included, into 0..1 across.
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, current := range order {
		minX, maxX = math.Min(minX, current.x-current.radius), math.Max(maxX, current.x+current.radius)
		minY, maxY = math.Min(minY, current.y-current.radius), math.Max(maxY, current.y+current.radius)
	}
	width := math.Max(maxX-minX, 1e-9)
	round := func(value float64) float32 { return float32(math.Round(value*10000) / 10000) }
	for _, spot := range all {
		points = append(points, Point{ID: spot.id, X: round((spot.x - minX) / width), Y: round((spot.y - minY) / width)})
	}
	islands := make([]Island, len(order))
	for index, current := range order {
		islands[index] = Island{Group: current.group, X: round((current.x - minX) / width), Y: round((current.y - minY) / width), Radius: round(current.radius / width)}
	}
	return Map{Points: points, Islands: islands, Aspect: round((maxY - minY) / width)}
}

// placeIslands puts each island where its average meaning falls among the
// others', then pushes overlapping islands apart.
func placeIslands(order []*island, size int) {
	if len(order) == 1 {
		return
	}
	// The groups' averages, less their own average, weighted by size so a
	// handful of tiny groups can't decide the axes.
	rows := make([][]float64, len(order))
	mean := make([]float64, size)
	total := 0.0
	for index, current := range order {
		weight := float64(len(current.members))
		total += weight
		for position, value := range current.centre {
			mean[position] += weight * value
		}
		rows[index] = current.centre
	}
	weighted := make([][]float64, len(order))
	for index, row := range rows {
		scale := math.Sqrt(float64(len(order[index].members)))
		weighted[index] = make([]float64, size)
		for position, value := range row {
			weighted[index][position] = (value - mean[position]/total) * scale
		}
	}
	first := component(weighted, size, nil)
	second := component(weighted, size, first)

	// Spread the starting places to about the size the islands will cover.
	area := 0.0
	for _, current := range order {
		area += current.radius * current.radius
	}
	xs, ys := make([]float64, len(order)), make([]float64, len(order))
	var spreadX, spreadY float64
	for index, row := range rows {
		centred := make([]float64, size)
		for position, value := range row {
			centred[position] = value - mean[position]/total
		}
		xs[index], ys[index] = dot(centred, first), dot(centred, second)
		spreadX, spreadY = math.Max(spreadX, math.Abs(xs[index])), math.Max(spreadY, math.Abs(ys[index]))
	}
	reach := math.Sqrt(area) * 1.2
	for index, current := range order {
		current.x = layoutWidth * reach * xs[index] / math.Max(spreadX, 1e-9)
		current.y = reach * ys[index] / math.Max(spreadY, 1e-9)
	}

	// Push apart, a little at a time, leaving a channel between islands;
	// a gentle pull toward the middle keeps the archipelago together.
	for round := 0; round < 400; round++ {
		moved := false
		for i := range order {
			for j := i + 1; j < len(order); j++ {
				a, b := order[i], order[j]
				gap := 0.18*(a.radius+b.radius) + 1
				need := a.radius + b.radius + gap
				dx, dy := b.x-a.x, b.y-a.y
				distance := math.Hypot(dx, dy)
				if distance >= need {
					continue
				}
				if distance < 1e-9 {
					// Exactly on top of each other: part them along a fixed direction.
					angle := float64(i*7+j*13) * 0.618
					dx, dy, distance = math.Cos(angle), math.Sin(angle), 1
				}
				push := (need - distance) / distance
				// The smaller island moves more.
				share := a.radius * a.radius / (a.radius*a.radius + b.radius*b.radius)
				a.x -= dx * push * (1 - share)
				a.y -= dy * push * (1 - share)
				b.x += dx * push * share
				b.y += dy * push * share
				moved = true
			}
		}
		for _, current := range order {
			current.x *= 0.995
			current.y *= 0.995
		}
		if !moved && round > 20 {
			break
		}
	}
}

type spot struct {
	id   int64
	x, y float64
}

// spreadIsland places an island's posts by their two main differences from
// the island's average, so the island takes the shape of its posts: dense
// where many are alike, thinning out toward the odd ones. Distances are
// scaled so most posts fall well inside the island and eased so the
// furthest still stay on it.
func spreadIsland(vectors *snapshot, current *island) []spot {
	count := len(current.members)
	spots := make([]spot, count)
	if count == 1 {
		spots[0] = spot{id: vectors.ids[current.members[0]], x: current.x, y: current.y}
		return spots
	}
	size := vectors.size
	rows := make([][]float64, count)
	for index, row := range current.members {
		norm := float64(vectors.lengths[row])
		values := make([]float64, size)
		for position, value := range vectors.vector(row) {
			values[position] = float64(value)/norm - current.centre[position]
		}
		rows[index] = values
	}
	first := component(rows, size, nil)
	second := component(rows, size, first)
	xs, ys, distances := make([]float64, count), make([]float64, count), make([]float64, count)
	for index, row := range rows {
		xs[index], ys[index] = dot(row, first), dot(row, second)
		distances[index] = math.Hypot(xs[index], ys[index])
	}
	sorted := append([]float64(nil), distances...)
	sort.Float64s(sorted)
	// The distance most posts are within sets the scale.
	typical := sorted[(len(sorted)-1)*85/100]
	if typical <= 0 {
		typical = 1
	}
	// A small group stays close together, so a few posts still read as a
	// group rather than strays; from about sixty posts it fills its island.
	reach := current.radius * (0.45 + 0.55*math.Min(1, float64(count)/60))
	for index := range rows {
		scale := 0.0
		if distances[index] > 0 {
			// tanh keeps everyone on the island: at the typical distance a
			// post sits about three quarters of the way out.
			scale = reach * math.Tanh(distances[index]/typical) / distances[index]
		}
		spots[index] = spot{id: vectors.ids[current.members[index]], x: current.x + xs[index]*scale, y: current.y + ys[index]*scale}
	}
	return spots
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
