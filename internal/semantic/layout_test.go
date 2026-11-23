package semantic

import (
	"context"
	"errors"
	"math"
	"testing"
)

// topics is the library the layout tests draw: three topics, four posts each.
var topics = map[string]string{
	"11": "My automobile engine", "12": "A vehicle with a new engine", "13": "Cars and more cars", "14": "The car show",
	"21": "Sourdough loaf", "22": "Baking bread with rye flour", "23": "A flour and sourdough guide", "24": "Bread baking",
	"31": "Retry with backoff", "32": "A worker queue", "33": "Queues and retry in a worker", "34": "Worker retry",
}

// grouping puts each post in the group named by its ID's first digit, as
// a main category would.
func grouping(f *fixture) map[int64]int64 {
	groups := map[int64]int64{}
	for post, id := range f.ids {
		groups[id] = int64(post[0] - '0')
	}
	return groups
}

func TestLayoutPutsPostsAboutTheSameThingTogether(t *testing.T) {
	f := library(t, topics)
	f.index(t)
	ctx := context.Background()

	drawn, err := f.service.Layout(ctx, grouping(f))
	if err != nil || len(drawn.Points) != 12 || len(drawn.Islands) != 3 {
		t.Fatalf("layout: %d points, %d groups, %v", len(drawn.Points), len(drawn.Islands), err)
	}
	if drawn.Aspect <= 0 || drawn.Aspect > 3 {
		t.Fatalf("aspect %v", drawn.Aspect)
	}
	at := map[int64]Point{}
	for _, point := range drawn.Points {
		if point.X < 0 || point.X > 1 || point.Y < 0 || point.Y > drawn.Aspect+1e-3 {
			t.Fatalf("post off the map: %+v (aspect %v)", point, drawn.Aspect)
		}
		at[point.ID] = point
	}
	// On average, posts on one topic sit closer to each other than to posts on another.
	group := grouping(f)
	var same, other, sameCount, otherCount float64
	for a, p := range at {
		for b, q := range at {
			if a >= b {
				continue
			}
			distance := math.Hypot(float64(p.X-q.X), float64(p.Y-q.Y))
			if group[a] == group[b] {
				same, sameCount = same+distance, sameCount+1
			} else {
				other, otherCount = other+distance, otherCount+1
			}
		}
	}
	if same/sameCount >= other/otherCount {
		t.Fatalf("same-topic posts are %.3f apart on average, other-topic %.3f", same/sameCount, other/otherCount)
	}
	// Each topic's posts are alike, so they form its cluster rather than mixed ground.
	for _, point := range drawn.Points {
		if point.Fringe {
			t.Fatalf("post %d is in mixed ground, though its neighbours share its topic", point.ID)
		}
	}

	// The same vectors and groups give the same map, worked out once.
	again, err := f.service.Layout(ctx, grouping(f))
	if err != nil || &again.Points[0] != &drawn.Points[0] {
		t.Fatalf("layout was not kept: %v", err)
	}
}

func TestLayoutMarksPostsAmongAnotherTopicAsFringe(t *testing.T) {
	f := library(t, topics)
	f.index(t)
	groups := grouping(f)
	// A bread post filed under cars sits among bread posts: mixed ground.
	groups[f.ids["24"]] = 1
	drawn, err := f.service.Layout(context.Background(), groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range drawn.Points {
		if point.ID == f.ids["24"] && !point.Fringe {
			t.Fatal("a post among another topic's posts was not marked fringe")
		}
	}
}

func TestLayoutLeavesOutPostsWithoutAGroupAndRedrawsWhenRegrouped(t *testing.T) {
	f := library(t, topics)
	f.index(t)
	ctx := context.Background()
	groups := grouping(f)
	delete(groups, f.ids["11"]) // archived, say
	drawn, err := f.service.Layout(ctx, groups)
	if err != nil || len(drawn.Points) != 11 {
		t.Fatalf("got %d points, %v", len(drawn.Points), err)
	}
	for _, point := range drawn.Points {
		if point.ID == f.ids["11"] {
			t.Fatal("a post without a group was placed")
		}
	}
	groups[f.ids["21"]] = 3
	regrouped, err := f.service.Layout(ctx, groups)
	if err != nil || &regrouped.Points[0] == &drawn.Points[0] {
		t.Fatalf("regrouping kept the old map: %v", err)
	}
}

func TestLayoutIsTheSameEachTimeItIsWorkedOut(t *testing.T) {
	f := library(t, topics)
	f.index(t)
	vectors, err := f.service.fresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first, second := archipelago(vectors, grouping(f)), archipelago(vectors, grouping(f))
	for index := range first.Points {
		if first.Points[index] != second.Points[index] {
			t.Fatalf("map moved: %+v then %+v", first.Points[index], second.Points[index])
		}
	}
}

func TestLayoutRespectsMeaningSearchTurnedOff(t *testing.T) {
	f := library(t, map[string]string{"1": "automobile"})
	f.index(t)
	ctx := context.Background()
	if err := f.store.SetMeaningSearch(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Layout(ctx, grouping(f)); !errors.Is(err, ErrOff) {
		t.Fatalf("got %v, want ErrOff", err)
	}
}

func TestLayoutOfAnEmptyLibrary(t *testing.T) {
	f := library(t, map[string]string{})
	drawn, err := f.service.Layout(context.Background(), map[int64]int64{})
	if err != nil || len(drawn.Points) != 0 || len(drawn.Islands) != 0 {
		t.Fatalf("got %+v %v", drawn, err)
	}
}
