package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ringsaturn/tzf"
)

// zoneFinder resolves an observation's coordinates to the time zone in effect
// there. eBird records a local date and time of day with no zone or offset, so
// the zone has to come from somewhere else; the export's coordinates are the
// most faithful source, since they're where you actually were.
type zoneFinder interface {
	zoneAt(lat, lon float64) (*time.Location, error)
}

// errNoZoneForCoords reports a coordinate the boundary data doesn't cover,
// which in practice means one that isn't on Earth. It is the only failure a
// caller should answer by falling back: every other error from a zoneFinder
// means the finder itself is broken, and a run that fell back on those would
// silently date the whole feed in the wrong zone.
var errNoZoneForCoords = errors.New("no time zone covers those coordinates")

// tzfZoneFinder resolves coordinates against the timezone-boundary-builder
// polygons that github.com/ringsaturn/tzf embeds in the binary. No network
// access is involved.
//
// The polygons are loaded on first use rather than at construction: loading
// costs roughly 300ms, and a run that never resolves a coordinate (an export
// with no usable ones, or a test that injects a different finder) shouldn't pay
// for it.
type tzfZoneFinder struct {
	once sync.Once
	f    tzf.F
	err  error

	// locations memoizes time.LoadLocation, which re-reads the zoneinfo data
	// on every call.
	locations map[string]*time.Location
}

func newTZFZoneFinder() *tzfZoneFinder {
	return &tzfZoneFinder{locations: make(map[string]*time.Location)}
}

// zoneAt implements zoneFinder. It returns an error for coordinates no time
// zone covers, which in practice means coordinates that aren't on Earth: the
// boundary data includes the nautical zones over open ocean.
func (t *tzfZoneFinder) zoneAt(lat, lon float64) (*time.Location, error) {
	t.once.Do(func() {
		// The full-precision polygons. The cheaper NewDefaultFinder trades
		// accuracy near boundaries for a faster load, which is the wrong trade
		// for a program that runs once per feed generation.
		t.f, t.err = tzf.NewFullFinder()
		if t.err != nil {
			t.err = fmt.Errorf("loading time zone boundaries: %w", t.err)
		}
	})
	if t.err != nil {
		return nil, t.err
	}

	// Note the argument order: tzf takes longitude first.
	name := t.f.GetTimezoneName(lon, lat)
	if name == "" {
		return nil, fmt.Errorf("%w: %v, %v", errNoZoneForCoords, lat, lon)
	}

	if loc, ok := t.locations[name]; ok {
		return loc, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("loading time zone %q: %w", name, err)
	}
	t.locations[name] = loc
	return loc, nil
}

// coord is a cache key: one birding location, as it appears in the export.
type coord struct {
	lat, lon float64
}

// zoneResult is a memoized lookup, successful or not.
type zoneResult struct {
	loc *time.Location
	err error
}

// cachingZoneFinder memoizes an underlying finder by coordinate. An export
// repeats the same handful of locations across many rows, and a polygon lookup
// costs far more than a map hit.
//
// It is not safe for concurrent use; the parser resolves zones one row at a
// time.
type cachingZoneFinder struct {
	inner zoneFinder
	cache map[coord]zoneResult
}

func newCachingZoneFinder(inner zoneFinder) *cachingZoneFinder {
	return &cachingZoneFinder{inner: inner, cache: make(map[coord]zoneResult)}
}

// zoneAt implements zoneFinder.
func (c *cachingZoneFinder) zoneAt(lat, lon float64) (*time.Location, error) {
	k := coord{lat: lat, lon: lon}
	if r, ok := c.cache[k]; ok {
		return r.loc, r.err
	}
	loc, err := c.inner.zoneAt(lat, lon)
	c.cache[k] = zoneResult{loc: loc, err: err}
	return loc, err
}

// defaultZoneFinder is the finder used in production: real boundary data,
// looked up once per distinct location.
func defaultZoneFinder() zoneFinder {
	return newCachingZoneFinder(newTZFZoneFinder())
}
