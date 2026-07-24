package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// -update rewrites the golden files from the current output. Run it after a
// deliberate change to the feed, then read the diff: it is the review.
var update = flag.Bool("update", false, "rewrite the golden feed files in testdata")

// TestGoldenFeeds renders the sample export in every format and compares the
// whole document against a checked-in copy.
//
// The other feed tests assert substrings, which say nothing about the parts
// nobody thought to name — an element that turns up empty, an attribute that
// stops being written, a date that changes format. The output document is what
// this program produces, so it's what's pinned.
func TestGoldenFeeds(t *testing.T) {
	fc := feedConfig{
		Count:            20,
		FallbackTimezone: "America/Detroit",
		fallbackLocation: mustLocation(t, "America/Detroit"),
		// Standing in for a personal location, so the golden files show what a
		// blocked sighting publishes: county and state, and no site name.
		LocationBlocklist: locationBlocklist{"Lincoln Twp. Park"},
		Feed: feedMeta{
			Title:       "Test Feed",
			Description: "A test feed.",
			Link:        "https://example.com/",
			FeedURL:     "https://example.com/feed.xml",
			Author:      "Jane Doe",
			Language:    "en-US",
		},
	}

	f, err := os.Open("testdata/sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	// A static finder, so the golden files don't depend on the boundary data
	// and the test doesn't pay to load it.
	obs, err := parseObservations(f, &staticZoneFinder{loc: fc.FallbackLocation()}, fc.FallbackLocation())
	if err != nil {
		t.Fatalf("parseObservations: %v", err)
	}
	feed := buildFeed(mostRecent(obs, fc.Count), fc)

	for _, tc := range []struct{ format, golden string }{
		{"rss", "golden.rss.xml"},
		{"atom", "golden.atom.xml"},
		{"json", "golden.json"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			got, err := renderFeed(feed, tc.format)
			if err != nil {
				t.Fatalf("renderFeed(%s): %v", tc.format, err)
			}
			path := filepath.Join("testdata", tc.golden)
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run `go test -update` to create it)", err)
			}
			if string(got) != string(want) {
				t.Errorf("%s output differs from %s; re-read it before running `go test -update`\n"+
					"--- got ---\n%s\n--- want ---\n%s", tc.format, path, got, want)
			}
		})
	}
}
