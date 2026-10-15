package semantic

import (
	"math"
	"sort"
)

// Quantize packs a vector into one signed byte per dimension, scaled so its
// largest value uses the full range. Only the direction of a vector matters
// for comparing meaning, and bytes keep it to within a percent or so at a
// quarter of the size. It returns nil for a vector with no direction.
func Quantize(values []float32) []byte {
	var peak float64
	for _, value := range values {
		peak = math.Max(peak, math.Abs(float64(value)))
	}
	if peak == 0 || math.IsNaN(peak) || math.IsInf(peak, 0) {
		return nil
	}
	packed := make([]byte, len(values))
	for index, value := range values {
		packed[index] = byte(int8(math.Round(float64(value) / peak * 127)))
	}
	return packed
}

func length(vector []int8) float32 {
	var sum int64
	for _, value := range vector {
		sum += int64(value) * int64(value)
	}
	return float32(math.Sqrt(float64(sum)))
}

func signed(packed []byte) []int8 {
	vector := make([]int8, len(packed))
	for index, value := range packed {
		vector[index] = int8(value)
	}
	return vector
}

// Hit is a post and how close it is, from -1 to 1.
type Hit struct {
	ID    int64
	Score float32
}

// snapshot is every post's vector at one moment, laid end to end so a
// search reads straight through memory. It is never changed once built.
type snapshot struct {
	count   int
	version int64
	size    int
	ids     []int64
	vectors []int8
	lengths []float32
	at      map[int64]int
}

func (s *snapshot) vector(index int) []int8 {
	return s.vectors[index*s.size : (index+1)*s.size]
}

// standOut is how far above the library's typical closeness a post must
// be to count as about the same thing, in units of the usual spread. Three
// is about as far as chance alone reaches in a library of a few thousand.
const standOut = 3

// minForSpread is the fewest posts the typical closeness is worked out from.
const minForSpread = 8

// nearest returns the posts that are about the same thing as the vector,
// closest first, leaving out the post with ID skip. How close "close" is
// depends on the model and the library, so no fixed score is used: a post
// must be within margin of the closest one, and stand clear of how close
// posts in this library typically are. A search for something the library
// has nothing on therefore finds nothing, rather than its least bad posts.
func (s *snapshot) nearest(query []int8, limit int, skip int64, margin float32) []Hit {
	if len(query) != s.size || limit < 1 {
		return nil
	}
	queryLength := length(query)
	if queryLength == 0 {
		return nil
	}
	hits := make([]Hit, 0, len(s.ids))
	for index, id := range s.ids {
		if id == skip || s.lengths[index] == 0 {
			continue
		}
		var dot int32
		for position, value := range s.vector(index) {
			dot += int32(value) * int32(query[position])
		}
		hits = append(hits, Hit{ID: id, Score: float32(dot) / (queryLength * s.lengths[index])})
	}
	if len(hits) == 0 {
		return nil
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID > hits[j].ID
	})
	floor := hits[0].Score - margin
	// bar is what a post must be above to stand clear of the library's bulk.
	bar := float32(math.Inf(-1))
	if len(hits) >= minForSpread {
		// The middle score and the middle distance from it describe the bulk
		// of the library without being pulled up by the few close posts.
		typical := hits[len(hits)/2].Score
		distances := make([]float64, len(hits))
		for index, hit := range hits {
			distances[index] = math.Abs(float64(hit.Score - typical))
		}
		sort.Float64s(distances)
		// 1.4826 turns a middle distance into a standard deviation.
		spread := float32(1.4826 * distances[len(distances)/2])
		bar = typical + standOut*spread
	}
	kept := hits[:0]
	for _, hit := range hits {
		if hit.Score < floor || hit.Score <= bar || len(kept) == limit {
			break
		}
		kept = append(kept, hit)
	}
	return kept
}
