package semantic

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestLayoutPutsPostsAboutTheSameThingTogether(t *testing.T) {
	f := library(t, map[string]string{
		"11": "My automobile engine", "12": "A vehicle with a new engine", "13": "Cars and more cars",
		"21": "Sourdough loaf", "22": "Baking bread with rye flour", "23": "A flour and sourdough guide",
		"31": "Retry with backoff", "32": "A worker queue", "33": "Queues and retry in a worker",
	})
	f.index(t)
	ctx := context.Background()

	points, err := f.service.Layout(ctx)
	if err != nil || len(points) != 9 {
		t.Fatalf("layout: %d points, %v", len(points), err)
	}
	at := map[int64]Point{}
	for _, point := range points {
		if point.X < 0 || point.X > 1 || point.Y < 0 || point.Y > 1 {
			t.Fatalf("point off the map: %+v", point)
		}
		at[point.ID] = point
	}
	distance := func(a, b string) float64 {
		p, q := at[f.ids[a]], at[f.ids[b]]
		return math.Hypot(float64(p.X-q.X), float64(p.Y-q.Y))
	}
	// Every post sits closer to the others on its topic than to any post on another.
	groups := [][]string{{"11", "12", "13"}, {"21", "22", "23"}, {"31", "32", "33"}}
	for g, group := range groups {
		for _, a := range group {
			far := math.Inf(1)
			for o, other := range groups {
				if o == g {
					continue
				}
				for _, b := range other {
					far = math.Min(far, distance(a, b))
				}
			}
			for _, b := range group {
				if a != b && distance(a, b) >= far {
					t.Fatalf("%s is no closer to %s (%.3f) than to another topic (%.3f)", a, b, distance(a, b), far)
				}
			}
		}
	}

	// The same vectors give the same map, worked out once.
	again, err := f.service.Layout(ctx)
	if err != nil || &again[0] != &points[0] {
		t.Fatalf("layout was not kept: %v", err)
	}
}

func TestLayoutIsTheSameEachTimeItIsWorkedOut(t *testing.T) {
	f := library(t, map[string]string{"1": "automobile engine", "2": "sourdough loaf", "3": "worker queue", "4": "vehicle"})
	f.index(t)
	vectors, err := f.service.fresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first, second := project(vectors), project(vectors)
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("map moved: %+v then %+v", first[index], second[index])
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
	if _, err := f.service.Layout(ctx); !errors.Is(err, ErrOff) {
		t.Fatalf("got %v, want ErrOff", err)
	}
}

func TestLayoutOfAnEmptyLibrary(t *testing.T) {
	f := library(t, map[string]string{})
	points, err := f.service.Layout(context.Background())
	if err != nil || len(points) != 0 {
		t.Fatalf("got %v %v", points, err)
	}
}
