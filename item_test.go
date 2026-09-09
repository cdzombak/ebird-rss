package main

import (
	"strings"
	"testing"
)

func TestObservationDescription(t *testing.T) {
	public := Observation{Location: "Lincoln Twp. Park", County: "Berrien", StateProvince: "US-MI"}
	private := Observation{Location: "1234 Sparrow Lane, Anytown, MI 99999", County: "Washtenaw", StateProvince: "US-MI"}
	bl := locationBlocklist{"Sparrow Lane"}

	if got, want := public.Description(nil), "Lincoln Twp. Park, Berrien, MI, US"; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
	if got, want := public.Description(bl), "Lincoln Twp. Park, Berrien, MI, US"; got != want {
		t.Errorf("unmatched location should survive the blocklist: %q, want %q", got, want)
	}
	if got, want := private.Description(bl), "Washtenaw, MI, US"; got != want {
		t.Errorf("blocked location: Description() = %q, want %q", got, want)
	}
	// Without the blocklist, that same sighting publishes the street address.
	if got, want := private.Description(nil), "1234 Sparrow Lane, Anytown, MI 99999, Washtenaw, MI, US"; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

// Missing fields are skipped rather than left as stray commas.
func TestObservationDescriptionPartialFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Observation
		want string
	}{
		{"no county", Observation{Location: "Gallup Park", StateProvince: "US-MI"}, "Gallup Park, MI, US"},
		{"no state", Observation{Location: "Gallup Park", County: "Washtenaw"}, "Gallup Park, Washtenaw"},
		{"location only", Observation{Location: "Gallup Park"}, "Gallup Park"},
		{"county and state only", Observation{County: "Washtenaw", StateProvince: "US-MI"}, "Washtenaw, MI, US"},
		{"nothing at all", Observation{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.o.Description(nil); got != tc.want {
				t.Errorf("Description() = %q, want %q", got, tc.want)
			}
		})
	}

	// A blocked location with nothing else to say leaves an empty description
	// rather than leaking the name.
	o := Observation{Location: "Home"}
	if got := o.Description(locationBlocklist{"Home"}); got != "" {
		t.Errorf("Description() = %q, want empty", got)
	}
}

// Either the breeding code or the observer's notes may be absent, so every
// combination of present and missing has to read correctly.
func TestObservationDescriptionSections(t *testing.T) {
	base := Observation{Location: "Gallup Park", County: "Washtenaw", StateProvince: "US-MI"}
	const where = "Gallup Park, Washtenaw, MI, US"

	withCode := base
	withCode.BreedingCode = "S Singing Bird"
	withDetails := base
	withDetails.Details = "Heard, not seen"
	both := withCode
	both.Details = "Heard, not seen"

	for _, tc := range []struct {
		name string
		o    Observation
		want string
	}{
		{"location only", base, where},
		{"breeding code", withCode, "S Singing Bird<br>" + where},
		{"details", withDetails, where + "<br><br>Heard, not seen"},
		{"both", both, "S Singing Bird<br>" + where + "<br><br>Heard, not seen"},
		// Whitespace-only columns count as absent.
		{"blank code and details", Observation{
			Location: "Gallup Park", County: "Washtenaw", StateProvince: "US-MI",
			BreedingCode: "  ", Details: "\t",
		}, where},
		// A note on a sighting whose location is blocked still reads correctly.
		{"blocked location", Observation{
			Location: "1234 Sparrow Lane", County: "Washtenaw", StateProvince: "US-MI",
			Details: "At the feeder",
		}, "Washtenaw, MI, US<br><br>At the feeder"},
		// Nothing to say about where: the note stands alone, with no leading
		// break.
		{"nothing but a note", Observation{Location: "1234 Sparrow Lane", Details: "At the feeder"},
			"At the feeder"},
		{"nothing but a code", Observation{Location: "1234 Sparrow Lane", BreedingCode: "S Singing Bird"},
			"S Singing Bird"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.o.Description(locationBlocklist{"Sparrow Lane"}); got != tc.want {
				t.Errorf("Description() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The export is free text; the only markup in a description is the line breaks
// this program puts there.
func TestObservationDescriptionEscapesExportText(t *testing.T) {
	o := Observation{
		Location:     "Smith & Sons Preserve",
		County:       "Washtenaw",
		BreedingCode: "P Pair <in> Suitable Habitat",
		Details:      `Chased off a Cooper's Hawk & a "crow"`,
	}
	want := "P Pair &lt;in&gt; Suitable Habitat<br>" +
		"Smith &amp; Sons Preserve, Washtenaw<br><br>" +
		"Chased off a Cooper&#39;s Hawk &amp; a &#34;crow&#34;"
	if got := o.Description(nil); got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

// An eBird note is multi-line free text, and the description is HTML: without
// converting the breaks, a note written as paragraphs renders as one long line.
func TestObservationDescriptionKeepsNoteLineBreaks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		details string
		want    string
	}{
		{"single break", "Heard first.\nSeen later.", "Heard first.<br>Seen later."},
		{"blank line", "Heard first.\n\nSeen later.", "Heard first.<br><br>Seen later."},
		{"crlf", "Heard first.\r\nSeen later.", "Heard first.<br>Seen later."},
		{"bare cr", "Heard first.\rSeen later.", "Heard first.<br>Seen later."},
		// The break is markup this program adds; the text around it is still
		// escaped.
		{"escaped either side", "a & b\nc < d", "a &amp; b<br>c &lt; d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Observation{Location: "Gallup Park", Details: tc.details}
			want := "Gallup Park<br><br>" + tc.want
			if got := o.Description(nil); got != want {
				t.Errorf("Description() = %q, want %q", got, want)
			}
		})
	}
}

// eBird writes State/Province largest-unit-first ("US-MI"); the feed reads
// narrowest-first ("MI, US") to match the "Location, County" ahead of it.
func TestFormatRegion(t *testing.T) {
	for _, tc := range []struct{ code, want string }{
		{"US-MI", "MI, US"},
		{"CA-ON", "ON, CA"},
		{"GB-ENG", "ENG, GB"},
		{"MX-ROO", "ROO, MX"},
		// Not hyphenated: left alone.
		{"US", "US"},
		{"Michigan", "Michigan"},
		{"", ""},
		// More than two parts still reverses, narrowest first.
		{"US-MI-161", "161, MI, US"},
		// Malformed codes don't produce stray commas.
		{"US-", "US"},
		{"-MI", "MI"},
		{"-", ""},
		{" US-MI ", "MI, US"},
	} {
		if got := formatRegion(tc.code); got != tc.want {
			t.Errorf("formatRegion(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

// On a protocol where the observer was counting, an uncounted or missing value
// still reads as "multiple".
func TestCountLabel(t *testing.T) {
	for _, protocol := range []string{
		"eBird - Traveling Count",
		"eBird - Stationary Count",
		// An export that names some other protocol, or none at all, is read the
		// same way: only a casual observation is treated as uncounted.
		"eBird - Area Count",
		"eBird - Historical",
		"",
	} {
		for _, tc := range []struct {
			count string
			want  string
		}{
			{"1", "1"},
			{"14", "14"},
			{"X", "multiple"},
			{"x", "multiple"},
			{" X ", "multiple"},
			{"", "multiple"},
		} {
			o := Observation{CommonName: "American Robin", Count: tc.count, Protocol: protocol}
			if got := o.CountLabel(); got != tc.want {
				t.Errorf("protocol %q: CountLabel(%q) = %q, want %q", protocol, tc.count, got, tc.want)
			}
			if got, want := o.Title(), "American Robin ("+tc.want+")"; got != want {
				t.Errorf("protocol %q: Title() with count %q = %q, want %q", protocol, tc.count, got, want)
			}
		}
	}
}

// A casual observation — Merlin, typically — writes "X" whether one bird was
// seen or fifty, so "multiple" would be an invention. Only a number the
// observer actually entered is published.
func TestCountLabelCasualObservation(t *testing.T) {
	for _, tc := range []struct {
		count     string
		want      string
		wantTitle string
	}{
		{"1", "1", "American Robin (1)"},
		{"14", "14", "American Robin (14)"},
		{"X", "", "American Robin"},
		{"x", "", "American Robin"},
		{" X ", "", "American Robin"},
		{"", "", "American Robin"},
	} {
		o := Observation{CommonName: "American Robin", Count: tc.count, Protocol: "eBird - Casual Observation"}
		if got := o.CountLabel(); got != tc.want {
			t.Errorf("CountLabel(%q) = %q, want %q", tc.count, got, tc.want)
		}
		if got := o.Title(); got != tc.wantTitle {
			t.Errorf("Title() with count %q = %q, want %q", tc.count, got, tc.wantTitle)
		}
	}
}

// The protocol is free text in the export; it's matched with the portal prefix
// stripped, and without regard to case or surrounding space.
func TestIsCasualObservation(t *testing.T) {
	for _, tc := range []struct {
		protocol string
		want     bool
	}{
		{"eBird - Casual Observation", true},
		{" eBird  -  Casual Observation ", true},
		{"ebird - casual observation", true},
		{"eBird - CASUAL OBSERVATION", true},
		// No portal prefix: compared whole.
		{"Casual Observation", true},
		{"eBird - Traveling Count", false},
		{"eBird - Stationary Count", false},
		{"eBird - Nocturnal Flight Call Count", false},
		{"", false},
		// Near misses stay counted, rather than silently losing their counts.
		{"eBird - Casual Observation Count", false},
		{"Casual", false},
	} {
		if got := (Observation{Protocol: tc.protocol}).isCasualObservation(); got != tc.want {
			t.Errorf("isCasualObservation(%q) = %v, want %v", tc.protocol, got, tc.want)
		}
	}
}

func TestObservationGUIDAndURL(t *testing.T) {
	// With no scientific name, the common name identifies the species.
	o := Observation{SubmissionID: "S1", CommonName: "American Robin"}
	if got, want := o.GUID(nil), "ebird:S1:American Robin"; got != want {
		t.Errorf("GUID() = %q, want %q", got, want)
	}
	// With no submission ID there's no checklist to link to.
	if got := (Observation{CommonName: "American Robin"}).ChecklistURL(nil); got != "" {
		t.Errorf("ChecklistURL() = %q, want empty", got)
	}
}

func TestObservationGUIDAndURLPrivateLocation(t *testing.T) {
	o := Observation{
		SubmissionID:   "S1",
		Location:       "1234 Sparrow Lane, Anytown, MI 99999",
		ScientificName: "Turdus migratorius",
	}
	bl := locationBlocklist{"Sparrow Lane"}

	if got := o.ChecklistURL(bl); got != "" {
		t.Errorf("ChecklistURL() = %q, want empty for a blocklisted location", got)
	}
	// Unblocked, the checklist still links normally.
	if got, want := o.ChecklistURL(nil), checklistURLPrefix+"S1"; got != want {
		t.Errorf("ChecklistURL() = %q, want %q", got, want)
	}

	guid := o.GUID(bl)
	if strings.Contains(guid, "S1") {
		t.Errorf("GUID() = %q, should not contain the submission ID for a blocklisted location", guid)
	}
	if want := "ebird:S1:Turdus migratorius"; guid == want {
		t.Errorf("GUID() = %q, want something other than the unblocked form %q", guid, want)
	}
	// Deterministic: the same checklist gets the same GUID across runs.
	if got, want := o.GUID(bl), guid; got != want {
		t.Errorf("GUID() = %q, want %q (deterministic)", got, want)
	}
	// Unblocked, the GUID carries the real submission ID.
	if got, want := o.GUID(nil), "ebird:S1:Turdus migratorius"; got != want {
		t.Errorf("GUID() = %q, want %q", got, want)
	}
}
