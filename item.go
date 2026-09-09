package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"slices"
	"strings"
)

// The presentation half of an Observation: the methods that turn a parsed
// export row into one feed item's title, description, link, and identity.
// observations.go turns the CSV into Observations; nothing here reads the
// export.

// countMultiple is the Count value eBird uses for "present, but not counted".
const countMultiple = "X"

// multipleLabel is how countMultiple is rendered in an item title.
const multipleLabel = "multiple"

// protocolCasual is eBird's "casual observation" protocol, as the export names
// it once the portal prefix ("eBird - ") is stripped. A sighting logged this
// way — through Merlin, say — carries no count at all, which the export writes
// as countMultiple; see CountLabel.
const protocolCasual = "Casual Observation"

// checklistURLPrefix is the public eBird URL for a checklist, by submission ID.
const checklistURLPrefix = "https://ebird.org/checklist/"

// Title is the observation's feed item title: the common name, followed by the
// count in parentheses when there is one to show. A sighting CountLabel has
// nothing to say about is titled by name alone.
func (o Observation) Title() string {
	label := o.CountLabel()
	if label == "" {
		return o.CommonName
	}
	return fmt.Sprintf("%s (%s)", o.CommonName, label)
}

// CountLabel renders the raw eBird count for display, or "" when the export's
// count says nothing worth publishing.
//
// On a count the observer kept — a stationary or traveling count, or anything
// else deliberate — an uncounted ("X") or missing value means the birds were
// there in some number nobody tallied, which reads as "multiple". A casual
// observation carries no count in the first place: recording one bird through
// Merlin writes the same "X", so reporting "multiple" there would invent a
// flock out of a single bird. Those get no count unless the observer entered an
// actual number.
func (o Observation) CountLabel() string {
	c := strings.TrimSpace(o.Count)
	if c != "" && !strings.EqualFold(c, countMultiple) {
		return c
	}
	if o.isCasualObservation() {
		return ""
	}
	return multipleLabel
}

// isCasualObservation reports whether this sighting came from eBird's "casual
// observation" protocol.
//
// The export qualifies the protocol with the portal that recorded it — "eBird -
// Casual Observation" — so the prefix is dropped before comparing; a value
// carrying no such prefix is compared whole.
func (o Observation) isCasualObservation() bool {
	p := strings.TrimSpace(o.Protocol)
	if _, after, found := strings.Cut(p, " - "); found {
		p = after
	}
	return strings.EqualFold(strings.TrimSpace(p), protocolCasual)
}

// ChecklistURL is the public eBird page for the checklist this observation came
// from, or "" if the export carried no submission ID.
//
// A checklist at a blocklisted location is left unlinked entirely: the
// checklist page shows the location's exact address, and feed readers fetch a
// link to generate a preview (its OpenGraph data includes that same address)
// whether or not a person ever clicks it.
func (o Observation) ChecklistURL(blocklist locationBlocklist) string {
	if o.SubmissionID == "" || o.isPrivateLocation(blocklist) {
		return ""
	}
	return checklistURLPrefix + o.SubmissionID
}

// isPrivateLocation reports whether this observation's location is one the
// blocklist hides.
func (o Observation) isPrivateLocation(blocklist locationBlocklist) bool {
	return blocklist.hides(o.Location)
}

// Description is the feed item's description, as HTML:
//
//	Breeding Code<br>
//	Location, County, State/Province<br>
//	<br>
//	Observation Details
//
// Either the breeding code or the observer's notes may be absent; whatever is
// missing is left out, along with the line break that would have followed it.
//
// Text from the export is free-form, so it goes through escapeLines: the only
// markup in the result is this function's own, plus the line breaks a
// multi-line note asked for.
func (o Observation) Description(blocklist locationBlocklist) string {
	lines := make([]string, 0, 2)
	if code := strings.TrimSpace(o.BreedingCode); code != "" {
		lines = append(lines, escapeLines(code))
	}
	if where := o.locationText(blocklist); where != "" {
		lines = append(lines, escapeLines(where))
	}
	desc := strings.Join(lines, "<br>")

	details := escapeLines(strings.TrimSpace(o.Details))
	if details == "" {
		return desc
	}
	if desc == "" {
		return details
	}
	return desc + "<br><br>" + details
}

// escapeLines escapes export text for HTML and carries its line breaks over.
// An eBird note is multi-line free text, so without this a note written as
// paragraphs would render as one run-on line.
func escapeLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}

// locationText is where the sighting happened, as
// "Location, County, State/Province" — e.g. "Arcadia Marsh, Manistee, MI, US".
//
// A site the blocklist matches is left out entirely, leaving "County,
// State/Province" — eBird location names are free text and a personal location
// is often a home address. Empty fields are skipped, so an export missing one
// doesn't produce a stray comma.
func (o Observation) locationText(blocklist locationBlocklist) string {
	parts := make([]string, 0, 3)
	if o.Location != "" && !o.isPrivateLocation(blocklist) {
		parts = append(parts, o.Location)
	}
	if o.County != "" {
		parts = append(parts, o.County)
	}
	if region := formatRegion(o.StateProvince); region != "" {
		parts = append(parts, region)
	}
	return strings.Join(parts, ", ")
}

// formatRegion renders eBird's State/Province code for a human. The export
// writes it largest-unit-first and hyphenated ("US-MI"), which reads backwards
// next to the "Location, County" that precedes it; reversing the parts continues
// narrowest-to-widest ("MI, US").
//
// A code with no hyphen comes through unchanged, as do empty segments in a
// malformed one.
func formatRegion(code string) string {
	segments := strings.Split(strings.TrimSpace(code), "-")
	slices.Reverse(segments)
	parts := make([]string, 0, len(segments))
	for _, s := range segments {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// GUID is a stable, unique identifier for the observation. It is not a URL: a
// checklist's URL is shared by every species on that checklist, so the species
// has to be part of the identity.
//
// For a checklist at a blocklisted location, the submission ID is replaced by
// a one-way hash of itself: the ID is a private detail once it can be turned
// into that checklist's page (and its address), same as the location name
// itself, even though nothing here renders it as a link.
func (o Observation) GUID(blocklist locationBlocklist) string {
	name := o.ScientificName
	if name == "" {
		name = o.CommonName
	}
	id := o.SubmissionID
	if o.isPrivateLocation(blocklist) {
		id = obscureSubmissionID(id)
	}
	return fmt.Sprintf("ebird:%s:%s", id, name)
}

// obscureSubmissionID stands in for a submission ID in a GUID without
// revealing the ID itself. It's deterministic, so the same checklist keeps the
// same GUID across runs.
func obscureSubmissionID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "private-" + hex.EncodeToString(sum[:8])
}
