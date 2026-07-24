package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigFull(t *testing.T) {
	path := writeConfig(t, `
count: 30
format: atom
fallback_timezone: America/Detroit
location_blocklist:
  - Home
  - Sparrow Lane
feed:
  title: My Birds
  description: Birds I saw
  link: https://example.com/
  feed_url: https://example.com/feed.xml
  author: Jane Doe
  language: en-US
`)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Count != 30 || cfg.Format != "atom" {
		t.Errorf("count/format = %d/%q", cfg.Count, cfg.Format)
	}
	if cfg.FallbackLocation().String() != "America/Detroit" {
		t.Errorf("location = %q, want America/Detroit", cfg.FallbackLocation())
	}
	if cfg.Feed.Title != "My Birds" || cfg.Feed.FeedURL != "https://example.com/feed.xml" ||
		cfg.Feed.Author != "Jane Doe" || cfg.Feed.Language != "en-US" {
		t.Errorf("feed metadata = %+v", cfg.Feed)
	}
	if len(cfg.LocationBlocklist) != 2 || !cfg.LocationBlocklist.hides("Home feeders") {
		t.Errorf("location_blocklist = %v", cfg.LocationBlocklist)
	}
}

// Omitting the blocklist hides nothing; it must not become a blocklist that
// matches everything.
func TestLoadConfigNoBlocklist(t *testing.T) {
	cfg, err := loadConfig(writeConfig(t, "count: 5\n"))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.LocationBlocklist) != 0 {
		t.Errorf("location_blocklist = %v, want empty", cfg.LocationBlocklist)
	}
	if cfg.LocationBlocklist.hides("Home") {
		t.Error("an omitted blocklist should hide nothing")
	}
}

func TestLoadConfigExampleFile(t *testing.T) {
	// The shipped example must stay loadable as the config format evolves.
	if _, err := loadConfig("config.example.yml"); err != nil {
		t.Errorf("loadConfig(config.example.yml): %v", err)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Only a title is set; everything else should fall back to a default.
	path := writeConfig(t, "feed:\n  title: Minimal\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Count != defaultCount {
		t.Errorf("count = %d, want default %d", cfg.Count, defaultCount)
	}
	if cfg.Format != defaultFormat {
		t.Errorf("format = %q, want default %q", cfg.Format, defaultFormat)
	}
	if cfg.Feed.Link != defaultFeedLink {
		t.Errorf("link = %q, want default %q", cfg.Feed.Link, defaultFeedLink)
	}
	if cfg.Feed.Description != defaultFeedDescription {
		t.Errorf("description = %q, want default %q", cfg.Feed.Description, defaultFeedDescription)
	}
	if cfg.FallbackLocation() != time.Local {
		t.Errorf("location = %q, want the local zone", cfg.FallbackLocation())
	}
}

// A zero-valued config (as built in tests) still reports a usable location.
func TestConfigLocationFallsBackToLocal(t *testing.T) {
	if (feedConfig{}).FallbackLocation() != time.Local {
		t.Error("zero feedConfig should report the local zone")
	}
}

func TestLoadConfigErrors(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"unknown key", "tittle: typo\n"},
		{"count below one", "count: -1\n"},
		{"invalid format", "format: xml\n"},
		{"unknown fallback timezone", "fallback_timezone: Mars/Olympus_Mons\n"},
		{"empty file", ""},
		{"malformed yaml", "count: [1, 2\n"},
		// An empty entry would match every location and silently hide them all.
		{"empty blocklist entry", "location_blocklist:\n  - Home\n  - \"\"\n"},
		{"whitespace blocklist entry", "location_blocklist:\n  - \"   \"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadConfig(writeConfig(t, tc.body)); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestLoadConfigMissing(t *testing.T) {
	if _, err := loadConfig(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Fatal("expected an error for a missing config, got nil")
	}
}
