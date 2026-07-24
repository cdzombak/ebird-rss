package main

import (
	"fmt"
	"testing"
	"time"
)

// staticZoneFinder resolves every coordinate to the same zone, or fails every
// lookup. Parser tests that don't care about geography use it so they never
// touch the real boundary data.
type staticZoneFinder struct {
	loc   *time.Location
	err   error
	calls int
}

func (f *staticZoneFinder) zoneAt(_, _ float64) (*time.Location, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.loc, nil
}

// coord identifies one birding location, as it appears in the export.
type coord struct {
	lat, lon float64
}

// tableZoneFinder resolves specific coordinates to specific zones, and fails
// for anything it doesn't know. It's how tests exercise an export that spans
// several time zones.
type tableZoneFinder struct {
	zones map[coord]*time.Location
	calls int
}

func newTableZoneFinder(t *testing.T, zones map[coord]string) *tableZoneFinder {
	t.Helper()
	f := &tableZoneFinder{zones: make(map[coord]*time.Location, len(zones))}
	for c, name := range zones {
		f.zones[c] = mustLocation(t, name)
	}
	return f
}

func (f *tableZoneFinder) zoneAt(lat, lon float64) (*time.Location, error) {
	f.calls++
	if loc, ok := f.zones[coord{lat: lat, lon: lon}]; ok {
		return loc, nil
	}
	return nil, fmt.Errorf("%w: %v, %v", errNoZoneForCoords, lat, lon)
}

// TestTZFZoneFinder exercises the real boundary data. It's the one test that
// pays tzf's load cost, so it's skipped under -short.
func TestTZFZoneFinder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: loading time zone boundaries is slow")
	}
	f := newTZFZoneFinder()

	for _, tc := range []struct {
		name     string
		lat, lon float64
		want     string
	}{
		// Michigan spans two zones: most of the state is Eastern, the four
		// western Upper Peninsula counties are Central.
		{"Grand Mere State Park, Berrien Co. MI", 42.003406, -86.541923, "America/Detroit"},
		{"Ironwood, Gogebic Co. MI", 46.454, -90.171, "America/Menominee"},
		{"Salt Lake City, UT", 40.7608, -111.891, "America/Denver"},
		// Florida spans two zones as well.
		{"Pensacola, FL", 30.4213, -87.2169, "America/Chicago"},
		{"Miami, FL", 25.7617, -80.1918, "America/New_York"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := f.zoneAt(tc.lat, tc.lon)
			if err != nil {
				t.Fatalf("zoneAt: %v", err)
			}
			if loc.String() != tc.want {
				t.Errorf("zoneAt(%v, %v) = %q, want %q", tc.lat, tc.lon, loc, tc.want)
			}
		})
	}

	// Coordinates that aren't on Earth resolve to nothing. (Points at sea do
	// resolve: the boundary data covers the nautical zones.)
	if _, err := f.zoneAt(999, 999); err == nil {
		t.Error("expected an error for out-of-range coordinates, got nil")
	}
	if _, err := f.zoneAt(30.0, -40.0); err != nil {
		t.Errorf("mid-ocean coordinates should resolve to a nautical zone: %v", err)
	}
}

// The finder loads its data lazily, so constructing one costs nothing.
func TestTZFZoneFinderIsLazy(t *testing.T) {
	f := newTZFZoneFinder()
	if f.f != nil {
		t.Error("boundary data loaded before the first lookup")
	}
}
