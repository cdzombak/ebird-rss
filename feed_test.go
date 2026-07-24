package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleConfig() feedConfig {
	return feedConfig{
		Count:            20,
		Format:           "rss",
		FallbackTimezone: "UTC",
		fallbackLocation: time.UTC,
		Feed: feedMeta{
			Title:       "Test Feed",
			Description: "A test feed.",
			Link:        "https://example.com/",
			FeedURL:     "https://example.com/feed.xml",
			Author:      "Jane Doe",
			Language:    "en-US",
		},
	}
}

func sampleObservations() []Observation {
	return []Observation{
		{
			SubmissionID:   "S2",
			CommonName:     "American Robin",
			ScientificName: "Turdus migratorius",
			Count:          "3",
			Location:       "Lincoln Twp. Park",
			County:         "Berrien",
			StateProvince:  "US-MI",
			BreedingCode:   "S Singing Bird",
			Details:        `Chased off a Cooper's Hawk & a "crow"`,
			ObservedAt:     time.Date(2026, 4, 26, 9, 36, 0, 0, time.UTC),
		},
		{
			// Uncounted, and with no time of day: noon, as the parser dates it.
			SubmissionID:   "S1",
			CommonName:     "Canada Goose",
			ScientificName: "Branta canadensis",
			Count:          "X",
			Location:       "Grand Mere",
			County:         "Berrien",
			StateProvince:  "US-MI",
			ObservedAt:     time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
		},
	}
}

func TestBuildFeed(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	feed := buildFeed(sampleObservations(), sampleConfig(), now)

	if len(feed.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(feed.Items))
	}
	// Channel metadata comes from the config.
	if feed.Title != "Test Feed" || feed.Link != "https://example.com/" ||
		feed.Description != "A test feed." || feed.FeedLink != "https://example.com/feed.xml" ||
		feed.Language != "en-US" {
		t.Errorf("channel metadata not taken from config: %+v", feed)
	}
	if len(feed.Authors) != 1 || feed.Authors[0].Name != "Jane Doe" {
		t.Errorf("author not taken from config: %+v", feed.Authors)
	}
	if feed.Generator != appName+" "+version {
		t.Errorf("generator = %q, want %q", feed.Generator, appName+" "+version)
	}

	if feed.Items[0].Title != "American Robin (3)" {
		t.Errorf("item title = %q, want %q", feed.Items[0].Title, "American Robin (3)")
	}
	if feed.Items[1].Title != "Canada Goose (multiple)" {
		t.Errorf("uncounted item title = %q, want %q", feed.Items[1].Title, "Canada Goose (multiple)")
	}
	if feed.Items[0].Link != "https://ebird.org/checklist/S2" {
		t.Errorf("item link = %q, want the checklist URL", feed.Items[0].Link)
	}
	if feed.Items[0].GUID != "ebird:S2:Turdus migratorius" {
		t.Errorf("GUID = %q", feed.Items[0].GUID)
	}
	// The description says where, with the breeding code above it and the
	// observer's note below; Content carries it too, so Atom and JSON get it.
	if want := "S Singing Bird<br>Lincoln Twp. Park, Berrien, MI, US<br><br>" +
		"Chased off a Cooper&#39;s Hawk &amp; a &#34;crow&#34;"; feed.Items[0].Description != want ||
		feed.Items[0].Content != want {
		t.Errorf("description/content = %q / %q, want %q",
			feed.Items[0].Description, feed.Items[0].Content, want)
	}
	// Each item's date is the observation's date and time.
	if feed.Items[0].PublishedParsed == nil ||
		!feed.Items[0].PublishedParsed.Equal(time.Date(2026, 4, 26, 9, 36, 0, 0, time.UTC)) {
		t.Errorf("published = %v, want the observation time", feed.Items[0].PublishedParsed)
	}
	if feed.Items[1].PublishedParsed == nil ||
		!feed.Items[1].PublishedParsed.Equal(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("published = %v, want the observation date at noon", feed.Items[1].PublishedParsed)
	}
	// The feed's update time is the newest observation, not `now`.
	if feed.UpdatedParsed == nil || !feed.UpdatedParsed.Equal(time.Date(2026, 4, 26, 9, 36, 0, 0, time.UTC)) {
		t.Errorf("updated = %v, want the newest observation time", feed.UpdatedParsed)
	}
}

func TestBuildFeedEmpty(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	feed := buildFeed(nil, sampleConfig(), now)

	if len(feed.Items) != 0 {
		t.Errorf("got %d items, want none", len(feed.Items))
	}
	// With no observation to date the feed, it falls back to now.
	if feed.UpdatedParsed == nil || !feed.UpdatedParsed.Equal(now) {
		t.Errorf("updated = %v, want now", feed.UpdatedParsed)
	}
}

func TestWriteFeedRSS(t *testing.T) {
	out := filepath.Join(t.TempDir(), "feed.xml")
	if err := writeFeed(buildFeed(sampleObservations(), sampleConfig(), time.Now()), "rss", out); err != nil {
		t.Fatalf("writeFeed: %v", err)
	}
	s := readFile(t, out)

	for _, want := range []string{
		"<title>American Robin (3)</title>",
		"<title>Canada Goose (multiple)</title>",
		"<link>https://ebird.org/checklist/S2</link>",
		`<guid isPermaLink="false">ebird:S2:Turdus migratorius</guid>`,
		"<description>S Singing Bird&lt;br&gt;Lincoln Twp. Park, Berrien, MI, US&lt;br&gt;&lt;br&gt;" +
			"Chased off a Cooper&amp;#39;s Hawk &amp;amp; a &amp;#34;crow&amp;#34;</description>",
		"<pubDate>Sun, 26 Apr 2026 09:36:00 +0000</pubDate>",
		"<language>en-US</language>",                // from config
		"<managingEditor>Jane Doe</managingEditor>", // author, from config
	} {
		if !strings.Contains(s, want) {
			t.Errorf("RSS output missing %q\n---\n%s", want, s)
		}
	}
}

func TestWriteFeedAtom(t *testing.T) {
	out := filepath.Join(t.TempDir(), "feed.atom")
	if err := writeFeed(buildFeed(sampleObservations(), sampleConfig(), time.Now()), "atom", out); err != nil {
		t.Fatalf("writeFeed: %v", err)
	}
	s := readFile(t, out)

	for _, want := range []string{
		"<id>ebird:S2:Turdus migratorius</id>", // GUID becomes the Atom entry id
		"American Robin (3)",
		"Lincoln Twp. Park, Berrien, MI, US",
		`href="https://ebird.org/checklist/S2"`,
		`href="https://example.com/feed.xml" rel="self"`, // feed_url -> rel=self
		"Jane Doe", // author name
		// The description is HTML, so it belongs in content, marked as such.
		`type="html">S Singing Bird&lt;br&gt;Lincoln Twp. Park`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Atom output missing %q\n---\n%s", want, s)
		}
	}
	// …and not in the summary, which Atom reads as plain text: a reader would
	// show the markup instead of rendering it.
	if strings.Contains(s, "<summary>S Singing Bird") {
		t.Errorf("Atom summary carries HTML that would be shown literally\n---\n%s", s)
	}
	// RFC 4287 requires an <updated> on every entry. There are two here, plus
	// one for the feed itself.
	if strings.Contains(s, "<updated></updated>") {
		t.Errorf("Atom entry has an empty <updated>, which is invalid\n---\n%s", s)
	}
	if n := strings.Count(s, "<updated>2026-04-26T09:36:00Z</updated>"); n != 2 {
		t.Errorf("got %d entries/feed dated from the newest observation, want 2 (feed + first entry)\n---\n%s", n, s)
	}
	if !strings.Contains(s, "<updated>2026-04-25T12:00:00Z</updated>") {
		t.Errorf("second entry's <updated> is not its observation date\n---\n%s", s)
	}
}

func TestWriteFeedJSON(t *testing.T) {
	out := filepath.Join(t.TempDir(), "feed.json")
	if err := writeFeed(buildFeed(sampleObservations(), sampleConfig(), time.Now()), "json", out); err != nil {
		t.Fatalf("writeFeed: %v", err)
	}

	var doc struct {
		Title       string `json:"title"`
		HomePageURL string `json:"home_page_url"`
		FeedURL     string `json:"feed_url"`
		Items       []struct {
			ID            string `json:"id"`
			URL           string `json:"url"`
			Title         string `json:"title"`
			ContentHTML   string `json:"content_html"`
			Summary       string `json:"summary"`
			DatePublished string `json:"date_published"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(readFile(t, out)), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// Channel metadata from the config.
	if doc.Title != "Test Feed" || doc.HomePageURL != "https://example.com/" ||
		doc.FeedURL != "https://example.com/feed.xml" {
		t.Errorf("JSON feed metadata = %q / %q / %q", doc.Title, doc.HomePageURL, doc.FeedURL)
	}
	if len(doc.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(doc.Items))
	}
	it := doc.Items[0]
	if it.ID != "ebird:S2:Turdus migratorius" {
		t.Errorf("id = %q, want the observation GUID", it.ID)
	}
	if it.URL != "https://ebird.org/checklist/S2" {
		t.Errorf("url = %q, want the checklist URL", it.URL)
	}
	if it.Title != "American Robin (3)" {
		t.Errorf("title = %q", it.Title)
	}
	if !strings.HasPrefix(it.DatePublished, "2026-04-26T09:36:00") {
		t.Errorf("date_published = %q, want the observation time", it.DatePublished)
	}
	if want := "S Singing Bird<br>Lincoln Twp. Park, Berrien, MI, US<br><br>" +
		"Chased off a Cooper&#39;s Hawk &amp; a &#34;crow&#34;"; it.ContentHTML != want {
		t.Errorf("content_html = %q, want %q", it.ContentHTML, want)
	}
	// content_html is the only field JSON Feed allows markup in; summary is
	// plain text, so a reader would show this feed's markup literally.
	if it.Summary != "" {
		t.Errorf("summary = %q, want it omitted: it would be shown as plain text", it.Summary)
	}
}

// The blocklist reaches the rendered feed, not just Observation.Description.
func TestBuildFeedAppliesLocationBlocklist(t *testing.T) {
	obs := []Observation{{
		SubmissionID:  "S1",
		CommonName:    "American Robin",
		Count:         "1",
		Location:      "1234 Sparrow Lane, Anytown, MI 99999",
		County:        "Washtenaw",
		StateProvince: "US-MI",
		ObservedAt:    time.Date(2026, 4, 26, 9, 36, 0, 0, time.UTC),
	}}
	fc := sampleConfig()
	fc.LocationBlocklist = locationBlocklist{"Sparrow Lane"}

	out, err := renderFeed(buildFeed(obs, fc, time.Now()), "rss")
	if err != nil {
		t.Fatalf("renderFeed: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "<description>Washtenaw, MI, US</description>") {
		t.Errorf("blocked location not replaced by county/state:\n%s", s)
	}
	for _, leak := range []string{"Sparrow Lane", "1234", "99999"} {
		if strings.Contains(s, leak) {
			t.Errorf("blocked location leaked %q into the feed:\n%s", leak, s)
		}
	}
}

func TestRenderFeedUnknownFormat(t *testing.T) {
	if _, err := renderFeed(buildFeed(sampleObservations(), sampleConfig(), time.Now()), "xml"); err == nil {
		t.Fatal("expected an error for an unknown format, got nil")
	}
}

func TestWriteFeedStdout(t *testing.T) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	writeErr := writeFeed(buildFeed(sampleObservations(), sampleConfig(), time.Now()), "rss", "-")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = orig

	if writeErr != nil {
		t.Fatalf("writeFeed: %v", writeErr)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<rss") || !strings.Contains(string(out), "American Robin (3)") {
		t.Errorf("stdout output doesn't look like the RSS feed:\n%s", out)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
