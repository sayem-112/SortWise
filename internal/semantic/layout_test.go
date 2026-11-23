package semantic

import (
	"context"
	"errors"
	"math"
	"testing"
)

// topics is the library the layout tests draw: three topics, three posts each.
var topics = map[string]string{
	"11": "My automobile engine", "12": "A vehicle with a new engine", "13": "Cars and more cars",
	"21": "Sourdough loaf", "22": "Baking bread with rye flour", "23": "A flour and sourdough guide",
	"31": "Retry with backoff", "32": "A worker queue", "33": "Queues and retry in a worker",
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

func TestLayoutDrawsAnIslandPerGroup(t *testing.T) {
	f := library(t, topics)
	f.index(t)
	ctx := context.Background()

	drawn, err := f.service.Layout(ctx, grouping(f))
	if err != nil || len(drawn.Points) != 9 || len(drawn.Islands) != 3 {
		t.Fatalf("layout: %d points, %d islands, %v", len(drawn.Points), len(drawn.Islands), err)
	}
	if drawn.Aspect <= 0 || drawn.Aspect > 1.5 {
		t.Fatalf("aspect %v", drawn.Aspect)
	}
	islands := map[int64]Island{}
	for _, island := range drawn.Islands {
		islands[island.Group] = island
	}
	// No two islands overlap.
	for _, a := range drawn.Islands {
		for _, b := range drawn.Islands {
			if a.Group < b.Group && math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)) < float64(a.Radius+b.Radius) {
				t.Fatalf("islands %d and %d overlap: %+v %+v", a.Group, b.Group, a, b)
			}
		}
	}
	// Every post sits on its own island, inside the map.
	group := grouping(f)
	for _, point := range drawn.Points {
		island := islands[group[point.ID]]
		if math.Hypot(float64(point.X-island.X), float64(point.Y-island.Y)) > float64(island.Radius)+1e-3 {
			t.Fatalf("post %d is off its island: %+v %+v", point.ID, point, island)
		}
		if point.X < 0 || point.X > 1 || point.Y < 0 || point.Y > drawn.Aspect+1e-3 {
			t.Fatalf("post off the map: %+v (aspect %v)", point, drawn.Aspect)
		}
	}

	// The same vectors and groups give the same map, worked out once.
	again, err := f.service.Layout(ctx, grouping(f))
	if err != nil || &again.Points[0] != &drawn.Points[0] {
		t.Fatalf("layout was not kept: %v", err)
	}
}

func TestLayoutLeavesOutPostsWithoutAGroupAndRedrawsWhenRegrouped(t *testing.T) {
	f := library(t, topics)
	f.index(t)
	ctx := context.Background()
	groups := grouping(f)
	delete(groups, f.ids["11"]) // archived, say
	drawn, err := f.service.Layout(ctx, groups)
	if err != nil || len(drawn.Points) != 8 {
		t.Fatalf("got %d points, %v", len(drawn.Points), err)
	}
	for _, point := range drawn.Points {
		if point.ID == f.ids["11"] {
			t.Fatal("a post without a group was placed")
		}
	}
	// Moving a post to another group gives a new map.
	groups[f.ids["21"]] = 3
	regrouped, err := f.service.Layout(ctx, groups)
	if err != nil || len(regrouped.Islands) != 2 && len(regrouped.Islands) != 3 || &regrouped.Points[0] == &drawn.Points[0] {
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
