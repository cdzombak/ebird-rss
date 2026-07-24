package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// defaultCount is the number of observations included when the config omits one.
	defaultCount = 20
	// defaultFormat is the feed format used when the config omits one.
	defaultFormat = "rss"
	// defaultFeedTitle and defaultFeedLink are fallbacks for an unconfigured feed.
	defaultFeedTitle = "eBird Sightings"
	defaultFeedLink  = "https://ebird.org/"
	// defaultFeedDescription is used when the config omits a description.
	defaultFeedDescription = "Recent bird sightings from eBird."
)

// validFormats is the set of accepted values for the config's `format` field.
var validFormats = map[string]bool{"rss": true, "atom": true, "json": true}

// feedConfig is the parsed -config YAML: how many observations to include, the
// output format, how to interpret the export's timestamps, and how to describe
// the resulting feed.
type feedConfig struct {
	Count    int      `yaml:"count"`
	Format   string   `yaml:"format"`
	Timezone string   `yaml:"timezone"`
	Feed     feedMeta `yaml:"feed"`

	// location is the parsed Timezone, filled in by loadConfig.
	location *time.Location
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

// Location is the time zone the export's dates and times are interpreted in.
func (fc feedConfig) Location() *time.Location {
	if fc.location == nil {
		return time.Local
	}
	return fc.location
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
	var cfg feedConfig
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
	if cfg.Count == 0 {
		cfg.Count = defaultCount
	}
	if cfg.Count < 1 {
		return feedConfig{}, fmt.Errorf("config %q: count must be at least 1", path)
	}
	if cfg.Format == "" {
		cfg.Format = defaultFormat
	}
	if !validFormats[cfg.Format] {
		return feedConfig{}, fmt.Errorf("config %q: format must be one of rss, atom, json (got %q)", path, cfg.Format)
	}
	// An eBird export records local dates and times with no zone or offset, so
	// the zone they're read in is a configuration choice; it defaults to the
	// machine's local time.
	if cfg.Timezone == "" {
		cfg.location = time.Local
	} else {
		loc, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			return feedConfig{}, fmt.Errorf("config %q: unknown timezone %q: %w", path, cfg.Timezone, err)
		}
		cfg.location = loc
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
