package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/mmcdole/gofeed/atom"
	"github.com/mmcdole/gofeed/rss"
)

// buildFeed constructs a universal gofeed.Feed from the given observations,
// which are expected to be ordered newest-first. Channel-level metadata (title,
// link, description, self URL, author, language) comes from the feed
// configuration.
//
// Each item's title is "Common Name (Count)", its description is where the
// sighting happened (minus any location the config's blocklist hides) plus the
// breeding code and observer's notes when the export has them, and its date is
// the observation's date and time. The link points at the eBird checklist the
// observation came from.
func buildFeed(obs []Observation, fc feedConfig, now time.Time) *gofeed.Feed {
	feed := &gofeed.Feed{
		Title:       fc.Feed.Title,
		Link:        fc.Feed.Link,
		Description: fc.Feed.Description,
		Generator:   fmt.Sprintf("%s %s", appName, version),
	}
	// FeedLink is rendered as rel="self" in Atom and as feed_url in JSON Feed.
	// (The RSS converter has no self-link field, so RSS output omits it.)
	if fc.Feed.FeedURL != "" {
		feed.FeedLink = fc.Feed.FeedURL
	}
	if fc.Feed.Language != "" {
		feed.Language = fc.Feed.Language
	}
	if fc.Feed.Author != "" {
		feed.Authors = []*gofeed.Person{{Name: fc.Feed.Author}}
	}

	var newest time.Time
	for _, o := range obs {
		observedAt := o.ObservedAt
		// Description and Content carry the same HTML: the RSS converter renders
		// Description as <description>, while the Atom and JSON converters render
		// Content, so setting both makes every output format carry it.
		desc := o.Description(fc.LocationBlocklist)
		item := &gofeed.Item{
			Title:           o.Title(),
			Link:            o.ChecklistURL(),
			GUID:            o.GUID(),
			Description:     desc,
			Content:         desc,
			Published:       observedAt.Format(time.RFC3339),
			PublishedParsed: &observedAt,
		}
		if observedAt.After(newest) {
			newest = observedAt
		}
		feed.Items = append(feed.Items, item)
	}

	if newest.IsZero() {
		newest = now
	}
	feed.Updated = newest.Format(time.RFC3339)
	feed.UpdatedParsed = &newest

	return feed
}

// ebirdRSSConverter wraps the default RSS converter to fix up two things the
// default gets wrong for this feed:
//
//   - Dates. RSS 2.0 dates are RFC 822 (as amended by RFC 1123); the converter
//     copies through whatever string the universal feed carries, which is
//     RFC 3339 (right for Atom and JSON Feed, wrong here).
//   - GUIDs. An observation's GUID identifies one species on one checklist and
//     is not a URL, so isPermaLink="false" is accurate. The default converter
//     leaves the attribute unset, which readers are supposed to read as "true".
type ebirdRSSConverter struct {
	gofeed.DefaultRSSConverter
}

func (c *ebirdRSSConverter) Convert(f *gofeed.Feed) (*rss.Feed, error) {
	rssFeed, err := c.DefaultRSSConverter.Convert(f)
	if err != nil {
		return nil, err
	}
	rssFeed.PubDate = rssDate(rssFeed.PubDateParsed, rssFeed.PubDate)
	rssFeed.LastBuildDate = rssDate(rssFeed.LastBuildDateParsed, rssFeed.LastBuildDate)
	for _, item := range rssFeed.Items {
		item.PubDate = rssDate(item.PubDateParsed, item.PubDate)
		if item.GUID != nil && item.GUID.Value != "" {
			item.GUID.IsPermalink = "false"
		}
	}
	return rssFeed, nil
}

// ebirdAtomConverter wraps the default Atom converter to drop the entry summary.
//
// An Atom <summary> with no type attribute is plain text, so the default
// converter — which copies the universal feed's Description into it — would have
// readers show this feed's markup literally. The same text is already in
// <content type="html">, where it renders, so the summary has nothing to add.
type ebirdAtomConverter struct {
	gofeed.DefaultAtomConverter
}

func (c *ebirdAtomConverter) Convert(f *gofeed.Feed) (*atom.Feed, error) {
	atomFeed, err := c.DefaultAtomConverter.Convert(f)
	if err != nil {
		return nil, err
	}
	for _, entry := range atomFeed.Entries {
		if entry.Content != nil {
			entry.Summary = ""
		}
	}
	return atomFeed, nil
}

// rssDate renders t in the format RSS 2.0 requires, falling back to the
// already-rendered string when there's no parsed time to work from.
func rssDate(t *time.Time, fallback string) string {
	if t == nil {
		return fallback
	}
	return t.Format(time.RFC1123Z)
}

// renderFeed renders the feed in the requested format.
func renderFeed(feed *gofeed.Feed, format string) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	switch format {
	case "rss":
		err = feed.RenderRSS(&buf, &ebirdRSSConverter{})
	case "atom":
		err = feed.RenderAtom(&buf, &ebirdAtomConverter{})
	case "json":
		err = feed.RenderJSON(&buf, nil)
	default:
		return nil, fmt.Errorf("unknown feed format %q", format)
	}
	if err != nil {
		return nil, fmt.Errorf("rendering %s feed: %w", format, err)
	}
	return buf.Bytes(), nil
}

// writeFeed renders the feed in the requested format and writes it to outFile.
// A regular path is written atomically, so a reader (or web server) never
// observes a partially written feed; the special path "-" writes to stdout.
func writeFeed(feed *gofeed.Feed, format, outFile string) error {
	out, err := renderFeed(feed, format)
	if err != nil {
		return err
	}
	if outFile == "-" {
		_, err := os.Stdout.Write(out)
		return err
	}
	return atomicWriteFile(outFile, out, 0o644)
}
