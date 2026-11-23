package semantic

import (
	"math"
	"math/rand"
	"sort"
)

// The library map's final shape comes from each post's nearest neighbours,
// the way UMAP lays out a data map: every post is pulled toward the posts
// most like it and pushed away from a few others picked at random, starting
// from the category islands. Posts about the same thing end up together
// whatever their category, so the map is one continuous landscape, and a
// category shows as a cluster only where its posts really are alike.

const (
	// neighbours is the most closest posts each post is pulled toward; a
	// small library uses fewer, a quarter of its posts, so neighbours stay
	// the close ones.
	neighbours = 12
	// exactUpTo is the most posts compared with their full vectors. Beyond
	// it, vectors are shrunk to sketchSize numbers first, which keeps which
	// posts are close to which while comparing far faster.
	exactUpTo  = 4000
	sketchSize = 256
	// epochs is how many passes the layout makes over every neighbour pair.
	epochs = 150
	// pushes is how many random posts each pull is balanced by.
	pushes = 5
	// spread is the size of the starting layout, in the units the forces
	// below are tuned for.
	spread = 10.0
)

// nearestPosts lists, for each of the snapshot rows, the rows most like it,
// closest first.
func nearestPosts(vectors *snapshot, rows []int) [][]int {
	var compare func(a, b int) float32
	if len(rows) <= exactUpTo {
		compare = func(a, b int) float32 {
			first, second := vectors.vector(rows[a]), vectors.vector(rows[b])
			var dot int32
			for position, value := range first {
				dot += int32(value) * int32(second[position])
			}
			return float32(dot) / (vectors.lengths[rows[a]] * vectors.lengths[rows[b]])
		}
	} else {
		sketches := sketch(vectors, rows)
		compare = func(a, b int) float32 {
			var sum float32
			for slot, value := range sketches[a] {
				sum += value * sketches[b][slot]
			}
			return sum
		}
	}
	type candidate struct {
		index int
		score float32
	}
	k := min(neighbours, max(2, len(rows)/4), len(rows)-1)
	result := make([][]int, len(rows))
	if k <= 0 {
		return result
	}
	for index := range rows {
		best := make([]candidate, 0, k+1)
		for other := range rows {
			if other == index {
				continue
			}
			score := compare(index, other)
			if len(best) == k && score <= best[k-1].score {
				continue
			}
			at := sort.Search(len(best), func(i int) bool { return best[i].score < score })
			best = append(best, candidate{})
			copy(best[at+1:], best[at:])
			best[at] = candidate{other, score}
			if len(best) > k {
				best = best[:k]
			}
		}
		for _, found := range best {
			result[index] = append(result[index], found.index)
		}
	}
	return result
}

// sketch shrinks each post's vector to sketchSize numbers by projecting it
// onto fixed random directions, which keeps which posts are close to which.
func sketch(vectors *snapshot, rows []int) [][]float32 {
	random := rand.New(rand.NewSource(7))
	directions := make([][]float32, sketchSize)
	for index := range directions {
		direction := make([]float32, vectors.size)
		for position := range direction {
			if random.Intn(2) == 0 {
				direction[position] = -1
			} else {
				direction[position] = 1
			}
		}
		directions[index] = direction
	}
	sketches := make([][]float32, len(rows))
	for index, row := range rows {
		values := make([]float32, sketchSize)
		vector := vectors.vector(row)
		var length float64
		for slot, direction := range directions {
			var sum float32
			for position, value := range vector {
				sum += float32(value) * direction[position]
			}
			values[slot] = sum
			length += float64(sum) * float64(sum)
		}
		if length > 0 {
			scale := float32(1 / math.Sqrt(length))
			for slot := range values {
				values[slot] *= scale
			}
		}
		sketches[index] = values
	}
	return sketches
}

// relax moves the posts (xs, ys) so neighbours sit together, by UMAP's
// attraction along neighbour pairs and repulsion from random posts, with a
// learning rate that cools to nothing. It is seeded, so the same input
// always gives the same map.
func relax(xs, ys []float64, near [][]int) {
	count := len(xs)
	if count < 3 {
		return
	}
	// Bring the starting layout to the size the forces expect.
	minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for index := range xs {
		minX, maxX = math.Min(minX, xs[index]), math.Max(maxX, xs[index])
		minY, maxY = math.Min(minY, ys[index]), math.Max(maxY, ys[index])
	}
	scale := 2 * spread / math.Max(math.Max(maxX-minX, maxY-minY), 1e-9)
	for index := range xs {
		xs[index] = (xs[index] - (minX+maxX)/2) * scale
		ys[index] = (ys[index] - (minY+maxY)/2) * scale
	}

	// No single step moves a post more than 4 units, as in UMAP.
	clip := func(value float64) float64 {
		if value > 4 {
			return 4
		}
		if value < -4 {
			return -4
		}
		return value
	}
	random := rand.New(rand.NewSource(11))
	for epoch := 0; epoch < epochs; epoch++ {
		rate := 1 - float64(epoch)/float64(epochs)
		for index, list := range near {
			for _, other := range list {
				dx, dy := xs[index]-xs[other], ys[index]-ys[other]
				distance := dx*dx + dy*dy
				// Pull, with UMAP's simplest curve (a = b = 1).
				if distance > 0 {
					pull := -2 / (1 + distance)
					xs[index] += rate * clip(pull*dx)
					ys[index] += rate * clip(pull*dy)
					xs[other] -= rate * clip(pull*dx)
					ys[other] -= rate * clip(pull*dy)
				}
				// Push: away from a few posts picked at random.
				for push := 0; push < pushes; push++ {
					far := random.Intn(count)
					if far == index {
						continue
					}
					dx, dy := xs[index]-xs[far], ys[index]-ys[far]
					distance := dx*dx + dy*dy
					repel := 2 / ((0.01 + distance) * (1 + distance))
					xs[index] += rate * clip(repel*dx)
					ys[index] += rate * clip(repel*dy)
				}
			}
		}
	}
}
