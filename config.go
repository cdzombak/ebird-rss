package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Values used for config fields the user omits.
const (
	defaultCount           = 20
	defaultFormat          = "rss"
	defaultFeedTitle       = "eBird Sightings"
	defaultFeedLink        = "https://ebird.org/"
	defaultFeedDescription = "Recent bird sightings from eBird."
)

// validFormats is the set of accepted values for the config's `format` field.
var validFormats = map[string]bool{"rss": true, "atom": true, "json": true}

// feedConfig is the parsed -config YAML.
type feedConfig struct {
	Count             int               `yaml:"count"`
	Format            string            `yaml:"format"`
	FallbackTimezone  string            `yaml:"fallback_timezone"`
	LocationBlocklist locationBlocklist `yaml:"location_blocklist"`
	Feed              feedMeta          `yaml:"feed"`

	// fallbackLocation is the parsed FallbackTimezone, filled in by loadConfig.
	fallbackLocation *time.Location
}

// feedMeta is the channel-level metadata written into the output feed.
type feedMeta struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Link        string `yaml:"link"`
	FeedURL     string `yaml:"feed_url"`
	Author      string `yaml:"author"`
	Language    string `yaml:"language"`
}

// FallbackLocation is the time zone used for observations whose coordinates
// can't be resolved to one. Sightings that do carry usable coordinates are dated
// in the zone of the place they were recorded; see zoneFinder.
func (fc feedConfig) FallbackLocation() *time.Location {
	if fc.fallbackLocation == nil {
		return time.Local
	}
	return fc.fallbackLocation
}

// newConfig is the configuration a config file is decoded over. Count is seeded
// here rather than defaulted afterwards because yaml.v3 overwrites only the
// fields a document actually contains: an omitted `count` keeps the default,
// while an explicit `count: 0` survives to be rejected as the invalid value it
// is. Fields whose zero value is already "unset" — the strings — are defaulted
// in applyConfigDefaults instead.
func newConfig() feedConfig {
	return feedConfig{Count: defaultCount}
}

// loadConfigOrDefaults loads the configuration at path, or returns one with
// every field at its default when path is empty. -config is optional: a run
// that doesn't need to describe its feed shouldn't need a file to say so.
func loadConfigOrDefaults(path string) (feedConfig, error) {
	if path == "" {
		return applyConfigDefaults(newConfig(), "")
	}
	return loadConfig(path)
}

// loadConfig reads, parses, and validates the feed configuration file. Unknown
// keys are rejected so typos surface as errors rather than being silently
// ignored. Omitted fields fall back to sensible defaults.
func loadConfig(path string) (feedConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return feedConfig{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	cfg := newConfig()
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return feedConfig{}, fmt.Errorf("config %q is empty; see config.example.yml", path)
		}
		return feedConfig{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	return applyConfigDefaults(cfg, path)
}

// applyConfigDefaults fills in omitted fields and validates the result.
func applyConfigDefaults(cfg feedConfig, path string) (feedConfig, error) {
	if cfg.Count < 1 {
		return feedConfig{}, fmt.Errorf("config %q: count must be at least 1", path)
	}
	if cfg.Format == "" {
		cfg.Format = defaultFormat
	}
	if !validFormats[cfg.Format] {
		return feedConfig{}, fmt.Errorf("config %q: format must be one of rss, atom, json (got %q)", path, cfg.Format)
	}
	if cfg.FallbackTimezone == "" {
		cfg.fallbackLocation = time.Local
	} else {
		loc, err := time.LoadLocation(cfg.FallbackTimezone)
		if err != nil {
			return feedConfig{}, fmt.Errorf("config %q: unknown fallback_timezone %q: %w", path, cfg.FallbackTimezone, err)
		}
		cfg.fallbackLocation = loc
	}
	// An empty entry is a substring of every location, so it would silently
	// hide all of them — exactly the opposite of a careful blocklist.
	for i, entry := range cfg.LocationBlocklist {
		if strings.TrimSpace(entry) == "" {
			return feedConfig{}, fmt.Errorf("config %q: location_blocklist entry %d is empty", path, i+1)
		}
	}
	if cfg.Feed.Title == "" {
		cfg.Feed.Title = defaultFeedTitle
	}
	if cfg.Feed.Link == "" {
		cfg.Feed.Link = defaultFeedLink
	}
	if cfg.Feed.Description == "" {
		cfg.Feed.Description = defaultFeedDescription
	}
	return cfg, nil
}
