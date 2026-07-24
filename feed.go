package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/mmcdole/gofeed/atom"
	"github.com/mmcdole/gofeed/json"
	"github.com/mmcdole/gofeed/rss"
)

// buildFeed constructs a universal gofeed.Feed from the given observations,
// which are expected to be ordered newest-first. Channel-level metadata comes
// from the feed configuration; each item's title, description, link, and date
// come from the observation.
//
// The feed's own date is the newest observation's. An export with no rows
// never reaches here — parseObservations rejects it — so there is no invented
// date for the case where there's nothing to date the feed from.
func buildFeed(obs []Observation, fc feedConfig) *gofeed.Feed {
	feed := &gofeed.Feed{
		Title:       fc.Feed.Title,
		Link:        fc.Feed.Link,
		Description: fc.Feed.Description,
		Generator:   fmt.Sprintf("%s %s", appName, version),
		// FeedLink is rendered as rel="self" in Atom and as feed_url in JSON
		// Feed. (The RSS converter has no self-link field, so RSS omits it.)
		FeedLink: fc.Feed.FeedURL,
		Language: fc.Feed.Language,
	}
	// Unlike the plain string fields, an empty author has to be left off
	// entirely rather than assigned: a Person with no name still renders.
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

	if !newest.IsZero() {
		feed.Updated = newest.Format(time.RFC3339)
		feed.UpdatedParsed = &newest
	}

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

// ebirdAtomConverter wraps the default Atom converter to fix up two things the
// default gets wrong for this feed:
//
//   - The entry summary. An Atom <summary> with no type attribute is plain
//     text, so the default converter — which copies the universal feed's
//     Description into it — would have readers show this feed's markup
//     literally. The same text is already in <content type="html">, where it
//     renders.
//   - The entry update time. RFC 4287 §4.1.2 requires an <updated> on every
//     entry, and the default converter leaves it empty because the universal
//     feed carries only a published date.
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
		// eBird doesn't record when a sighting was last edited, so the
		// observation's own date stands in for it.
		if entry.Updated == "" {
			entry.Updated = entry.Published
			entry.UpdatedParsed = entry.PublishedParsed
		}
	}
	return atomFeed, nil
}

// ebirdJSONConverter wraps the default JSON Feed converter to drop the item
// summary, for the same reason ebirdAtomConverter does: JSON Feed defines
// summary as plain text — content_html is the one field the format allows
// markup in — and the default converter copies the universal feed's
// Description into both.
type ebirdJSONConverter struct {
	gofeed.DefaultJSONConverter
}

func (c *ebirdJSONConverter) Convert(f *gofeed.Feed) (*json.Feed, error) {
	jsonFeed, err := c.DefaultJSONConverter.Convert(f)
	if err != nil {
		return nil, err
	}
	for _, item := range jsonFeed.Items {
		if item.ContentHTML != "" {
			item.Summary = ""
		}
	}
	return jsonFeed, nil
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
		err = feed.RenderJSON(&buf, &ebirdJSONConverter{})
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
