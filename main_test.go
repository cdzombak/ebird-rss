package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// discardLogger keeps test output quiet.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestValidateArgs(t *testing.T) {
	full := cliArgs{inFile: "in.csv", outFile: "out.xml", configPath: "config.yml"}
	if err := validateArgs(full); err != nil {
		t.Errorf("complete args: %v", err)
	}
	for _, tc := range []struct {
		name string
		args cliArgs
	}{
		{"no in-file", cliArgs{outFile: "out.xml", configPath: "config.yml"}},
		{"no out-file", cliArgs{inFile: "in.csv", configPath: "config.yml"}},
		{"no config", cliArgs{inFile: "in.csv", outFile: "out.xml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateArgs(tc.args); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestGenerateFeed(t *testing.T) {
	cfg := sampleConfig()
	cfg.Count = 3
	cfg.fallbackLocation = mustLocation(t, "America/Detroit")

	out := filepath.Join(t.TempDir(), "feed.xml")
	err := generateFeed("testdata/sample.csv", out, cfg, &staticZoneFinder{loc: cfg.FallbackLocation()}, time.Now(), discardLogger())
	if err != nil {
		t.Fatalf("generateFeed: %v", err)
	}
	s := readFile(t, out)

	// The three most recent of the sample export's five observations.
	for _, want := range []string{
		"<title>American Robin (14)</title>",
		"<title>Canada Goose (2)</title>",
		"<title>Canada Goose (multiple)</title>",
		"<pubDate>Sun, 26 Apr 2026 09:36:00 -0400</pubDate>", // in the observation's zone
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\n---\n%s", want, s)
		}
	}
	// Older observations are excluded by the count.
	for _, dontWant := range []string{"Wood Duck", "Northern Flicker"} {
		if strings.Contains(s, dontWant) {
			t.Errorf("output should not include %q (beyond count=3)\n---\n%s", dontWant, s)
		}
	}
	if n := strings.Count(s, "<item>"); n != 3 {
		t.Errorf("got %d items, want 3", n)
	}
}

func TestGenerateFeedMissingInput(t *testing.T) {
	err := generateFeed(filepath.Join(t.TempDir(), "nope.csv"), "-", sampleConfig(), &staticZoneFinder{loc: time.UTC}, time.Now(), discardLogger())
	if err == nil {
		t.Fatal("expected an error for a missing input file, got nil")
	}
	if !strings.Contains(err.Error(), "open eBird export") {
		t.Errorf("error = %q, want it to mention opening the export", err)
	}
}

func TestGenerateFeedBadInput(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "notebird.csv")
	if err := os.WriteFile(bad, []byte("hello,world\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := generateFeed(bad, "-", sampleConfig(), &staticZoneFinder{loc: time.UTC}, time.Now(), discardLogger())
	if err == nil {
		t.Fatal("expected an error for a non-eBird CSV, got nil")
	}
	if !strings.Contains(err.Error(), "missing column(s)") {
		t.Errorf("error = %q, want it to name the missing columns", err)
	}
}

// TestRunLoadsConfig covers the whole path a real invocation takes, including
// the production zone finder resolving the sample export's coordinates.
func TestRunLoadsConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: loading time zone boundaries is slow")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(configPath, []byte("count: 2\nformat: json\nfallback_timezone: UTC\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "feed.json")

	args := cliArgs{inFile: "testdata/sample.csv", outFile: out, configPath: configPath}
	if err := run(args, discardLogger()); err != nil {
		t.Fatalf("run: %v", err)
	}

	s := readFile(t, out)
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		t.Errorf("output is not JSON:\n%s", s)
	}
	if n := strings.Count(s, `"title"`); n != 3 { // feed title + 2 items
		t.Errorf("got %d titles, want 3 (feed + 2 items)\n%s", n, s)
	}
}

func TestRunMissingConfig(t *testing.T) {
	args := cliArgs{inFile: "testdata/sample.csv", outFile: "-", configPath: filepath.Join(t.TempDir(), "nope.yml")}
	if err := run(args, discardLogger()); err == nil {
		t.Fatal("expected an error for a missing config, got nil")
	}
}

func TestAtomicWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feed.xml")
	if err := atomicWriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	if got := readFile(t, path); got != "first" {
		t.Errorf("contents = %q, want %q", got, "first")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("perm = %o, want 644", perm)
	}

	// A second write replaces the file, and leaves no temp files behind.
	if err := atomicWriteFile(path, []byte("second"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile (rewrite): %v", err)
	}
	if got := readFile(t, path); got != "second" {
		t.Errorf("contents = %q, want %q", got, "second")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want just the feed", len(entries))
	}
}

func TestAtomicWriteFileBadDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist", "feed.xml")
	if err := atomicWriteFile(path, []byte("x"), 0o644); err == nil {
		t.Fatal("expected an error writing into a missing directory, got nil")
	}
}
