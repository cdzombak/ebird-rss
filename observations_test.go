package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// sampleHeader is the header row of an eBird "MyEBirdData.csv" export.
const sampleHeader = "Submission ID,Common Name,Scientific Name,Taxonomic Order,Count,State/Province,County," +
	"Location ID,Location,Latitude,Longitude,Date,Time,Protocol,Duration (Min),All Obs Reported," +
	"Distance Traveled (km),Area Covered (ha),Number of Observers,Breeding Code,Observation Details," +
	"Checklist Comments,ML Catalog Numbers\n"

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

// mustParseObservations parses csv with every coordinate resolving to loc,
// which is also the fallback: the single-zone case.
func mustParseObservations(t *testing.T, csv string, loc *time.Location) []Observation {
	t.Helper()
	obs, err := parseObservations(strings.NewReader(csv), &staticZoneFinder{loc: loc}, loc)
	if err != nil {
		t.Fatalf("parseObservations: %v", err)
	}
	return obs
}

func TestParseObservationsSampleExport(t *testing.T) {
	f, err := os.Open("testdata/sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	loc := mustLocation(t, "America/Detroit")
	obs, err := parseObservations(f, &staticZoneFinder{loc: loc}, loc)
	if err != nil {
		t.Fatalf("parseObservations: %v", err)
	}
	if len(obs) != 5 {
		t.Fatalf("got %d observations, want 5", len(obs))
	}

	// Sorted newest first; the two species on checklist S327776301 share a
	// timestamp and are broken apart by common name.
	wantTitles := []string{
		"American Robin (14)",
		"Canada Goose (2)",
		"Canada Goose (multiple)",
		"Northern Flicker (Yellow-shafted) (1)",
		"Wood Duck (multiple)",
	}
	for i, want := range wantTitles {
		if got := obs[i].Title(); got != want {
			t.Errorf("obs[%d].Title() = %q, want %q", i, got, want)
		}
	}

	first := obs[0]
	if want := time.Date(2026, 4, 26, 9, 36, 0, 0, loc); !first.ObservedAt.Equal(want) {
		t.Errorf("obs[0].ObservedAt = %s, want %s", first.ObservedAt, want)
	}
	if first.Location != "Grand Mere State Park, Stevensville US-MI 42.00341, -86.54192" {
		t.Errorf("quoted location not parsed: %q", first.Location)
	}
	if first.County != "Berrien" || first.StateProvince != "US-MI" {
		t.Errorf("county/state = %q/%q, want Berrien/US-MI", first.County, first.StateProvince)
	}
	if got, want := first.Details, "Feeding on the lawn, bold & unbothered"; got != want {
		t.Errorf("obs[0].Details = %q, want %q", got, want)
	}
	if first.BreedingCode != "" {
		t.Errorf("obs[0].BreedingCode = %q, want empty", first.BreedingCode)
	}
	if got, want := first.Description(nil),
		"Grand Mere State Park, Stevensville US-MI 42.00341, -86.54192, Berrien, MI, US<br><br>"+
			"Feeding on the lawn, bold &amp; unbothered"; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
	// The other species on that checklist carries a breeding code and no note.
	if got, want := obs[1].BreedingCode, "H In Appropriate Habitat"; got != want {
		t.Errorf("obs[1].BreedingCode = %q, want %q", got, want)
	}
	if got, want := obs[1].Description(nil),
		"H In Appropriate Habitat<br>"+
			"Grand Mere State Park, Stevensville US-MI 42.00341, -86.54192, Berrien, MI, US"; got != want {
		t.Errorf("obs[1].Description() = %q, want %q", got, want)
	}
	if got, want := first.ChecklistURL(), "https://ebird.org/checklist/S327776301"; got != want {
		t.Errorf("ChecklistURL() = %q, want %q", got, want)
	}
	if got, want := first.GUID(), "ebird:S327776301:Turdus migratorius"; got != want {
		t.Errorf("GUID() = %q, want %q", got, want)
	}

	// The Wood Duck row has no Time and a short record (trailing columns
	// omitted); it lands at noon local.
	last := obs[len(obs)-1]
	if want := time.Date(2023, 10, 7, 12, 0, 0, 0, loc); !last.ObservedAt.Equal(want) {
		t.Errorf("timeless observation = %s, want %s", last.ObservedAt, want)
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

func TestParseObservationsTimeOfDay(t *testing.T) {
	loc := mustLocation(t, "America/Detroit")
	csv := sampleHeader +
		"S1,Morning Bird,Aves matutina,1,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,09:05 AM,eBird - Casual Observation,,0,,,1\n" +
		"S2,Noon Bird,Aves meridiana,2,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,12:00 PM,eBird - Casual Observation,,0,,,1\n" +
		"S3,Midnight Bird,Aves nocturna,3,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,12:00 AM,eBird - Casual Observation,,0,,,1\n" +
		"S4,Evening Bird,Aves vespertina,4,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,06:15 PM,eBird - Casual Observation,,0,,,1\n"

	obs := mustParseObservations(t, csv, loc)
	want := []struct {
		name string
		hour int
		min  int
	}{
		{"Evening Bird", 18, 15},
		{"Noon Bird", 12, 0},
		{"Morning Bird", 9, 5},
		{"Midnight Bird", 0, 0},
	}
	if len(obs) != len(want) {
		t.Fatalf("got %d observations, want %d", len(obs), len(want))
	}
	for i, w := range want {
		if obs[i].CommonName != w.name {
			t.Fatalf("obs[%d] = %q, want %q (wrong sort order)", i, obs[i].CommonName, w.name)
		}
		if h, m := obs[i].ObservedAt.Hour(), obs[i].ObservedAt.Minute(); h != w.hour || m != w.min {
			t.Errorf("%s parsed as %02d:%02d, want %02d:%02d", w.name, h, m, w.hour, w.min)
		}
	}
}

// A checklist with no time of day is dated to noon, so it sorts among that
// day's timed sightings instead of ahead of all of them.
func TestParseObservationsDatesTimelessChecklistsAtNoon(t *testing.T) {
	loc := mustLocation(t, "America/Detroit")
	csv := sampleHeader +
		"S1,Dawn Bird,Aves matutina,1,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,06:00 AM,eBird - Traveling Count,,0,,,1\n" +
		"S2,Timeless Bird,Aves incerta,2,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,,eBird - Casual Observation,,0,,,1\n" +
		"S3,Dusk Bird,Aves vespertina,3,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,08:30 PM,eBird - Traveling Count,,0,,,1\n" +
		// Days that gain and lose an hour to DST: noon is constructed, not
		// midnight plus twelve hours, so both still land at 12:00 local.
		"S4,Spring Forward Bird,Aves verna,4,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-03-08,,eBird - Casual Observation,,0,,,1\n" +
		"S5,Fall Back Bird,Aves autumnalis,5,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-11-01,,eBird - Casual Observation,,0,,,1\n"

	obs := mustParseObservations(t, csv, loc)
	byName := make(map[string]Observation, len(obs))
	for _, o := range obs {
		byName[o.CommonName] = o
	}

	for _, name := range []string{"Timeless Bird", "Spring Forward Bird", "Fall Back Bird"} {
		o := byName[name]
		if h, m := o.ObservedAt.Hour(), o.ObservedAt.Minute(); h != 12 || m != 0 {
			t.Errorf("%s dated %02d:%02d local, want 12:00", name, h, m)
		}
	}

	// Newest first: the timeless checklist falls between the day's two timed
	// ones rather than below both.
	wantOrder := []string{"Fall Back Bird", "Dusk Bird", "Timeless Bird", "Dawn Bird", "Spring Forward Bird"}
	for i, want := range wantOrder {
		if obs[i].CommonName != want {
			t.Errorf("obs[%d] = %q, want %q", i, obs[i].CommonName, want)
		}
	}
}

// An export spanning several time zones is dated per location, not per file.
func TestParseObservationsUsesPerLocationZones(t *testing.T) {
	// Same wall-clock date and time, three places. Ordered by absolute instant,
	// the westernmost is newest.
	csv := sampleHeader +
		"S1,Detroit Bird,Aves detroitiensis,1,1,US-MI,Wayne,L1,Belle Isle,42.34,-82.98,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n" +
		"S2,Denver Bird,Aves coloradensis,2,1,US-CO,Denver,L2,City Park,39.75,-104.95,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n" +
		"S3,Menominee Bird,Aves menomineeensis,3,1,US-MI,Gogebic,L3,Ironwood,46.45,-90.17,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n"

	finder := newTableZoneFinder(t, map[coord]string{
		{lat: 42.34, lon: -82.98}:  "America/Detroit",   // Eastern
		{lat: 39.75, lon: -104.95}: "America/Denver",    // Mountain
		{lat: 46.45, lon: -90.17}:  "America/Menominee", // Central, in Michigan
	})
	obs, err := parseObservations(strings.NewReader(csv), finder, time.UTC)
	if err != nil {
		t.Fatalf("parseObservations: %v", err)
	}
	if len(obs) != 3 {
		t.Fatalf("got %d observations, want 3", len(obs))
	}

	// 09:00 local in each zone: 13:00, 14:00, and 15:00 UTC respectively.
	wantUTCHour := map[string]int{"Detroit Bird": 13, "Menominee Bird": 14, "Denver Bird": 15}
	for _, o := range obs {
		if got, want := o.ObservedAt.UTC().Hour(), wantUTCHour[o.CommonName]; got != want {
			t.Errorf("%s: %02d:00 UTC, want %02d:00", o.CommonName, got, want)
		}
		if o.ZoneFallback {
			t.Errorf("%s: fell back to the configured zone", o.CommonName)
		}
	}
	// Newest first, by instant — which is the reverse of what a single-zone
	// reading of this file would produce.
	wantOrder := []string{"Denver Bird", "Menominee Bird", "Detroit Bird"}
	for i, want := range wantOrder {
		if obs[i].CommonName != want {
			t.Errorf("obs[%d] = %q, want %q", i, obs[i].CommonName, want)
		}
	}
}

func TestParseObservationsFallsBackWithoutCoordinates(t *testing.T) {
	csv := sampleHeader +
		"S1,American Robin,Turdus migratorius,29740,1,US-MI,Wayne,L1,Belle Isle,,,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n"

	fallback := mustLocation(t, "America/Detroit")
	finder := &staticZoneFinder{loc: time.UTC}
	obs, err := parseObservations(strings.NewReader(csv), finder, fallback)
	if err != nil {
		t.Fatalf("parseObservations: %v", err)
	}
	if finder.calls != 0 {
		t.Errorf("finder consulted %d times for a row with no coordinates, want 0", finder.calls)
	}
	if !obs[0].ZoneFallback {
		t.Error("ZoneFallback = false, want true")
	}
	if got := obs[0].ObservedAt.UTC().Hour(); got != 13 { // 09:00 EDT
		t.Errorf("observation dated %02d:00 UTC, want 13:00 (the fallback zone)", got)
	}
}

// A coordinate the finder can't place shouldn't fail the whole run; the row
// falls back and says so.
func TestParseObservationsFallsBackWhenLookupFails(t *testing.T) {
	csv := sampleHeader +
		"S1,American Robin,Turdus migratorius,29740,1,XX,,L1,Nowhere,999,999,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n"

	fallback := mustLocation(t, "America/Detroit")
	obs, err := parseObservations(strings.NewReader(csv), &staticZoneFinder{err: errNoZoneForCoords}, fallback)
	if err != nil {
		t.Fatalf("parseObservations: %v", err)
	}
	if !obs[0].ZoneFallback {
		t.Error("ZoneFallback = false, want true")
	}
	if got := obs[0].ObservedAt.UTC().Hour(); got != 13 { // 09:00 EDT
		t.Errorf("observation dated %02d:00 UTC, want 13:00 (the fallback zone)", got)
	}
}

// A finder that's broken rather than merely stumped fails the run. Falling
// back would date every row in the fallback zone and publish it as if the
// zones had been resolved.
func TestParseObservationsFailsWhenTheFinderIsBroken(t *testing.T) {
	csv := sampleHeader +
		"S1,American Robin,Turdus migratorius,29740,1,US-MI,Berrien,L1,Lincoln Twp. Park,42.0,-86.5,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n"

	broken := errors.New("loading time zone boundaries: no space left on device")
	_, err := parseObservations(strings.NewReader(csv), &staticZoneFinder{err: broken}, time.UTC)
	if !errors.Is(err, broken) {
		t.Fatalf("parseObservations error = %v, want it to report %v", err, broken)
	}
}

func TestCountLabel(t *testing.T) {
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
		o := Observation{CommonName: "American Robin", Count: tc.count}
		if got := o.CountLabel(); got != tc.want {
			t.Errorf("CountLabel(%q) = %q, want %q", tc.count, got, tc.want)
		}
		if got, want := o.Title(), "American Robin ("+tc.want+")"; got != want {
			t.Errorf("Title() with count %q = %q, want %q", tc.count, got, want)
		}
	}
}

func TestObservationGUIDAndURL(t *testing.T) {
	// With no scientific name, the common name identifies the species.
	o := Observation{SubmissionID: "S1", CommonName: "American Robin"}
	if got, want := o.GUID(), "ebird:S1:American Robin"; got != want {
		t.Errorf("GUID() = %q, want %q", got, want)
	}
	// With no submission ID there's no checklist to link to.
	if got := (Observation{CommonName: "American Robin"}).ChecklistURL(); got != "" {
		t.Errorf("ChecklistURL() = %q, want empty", got)
	}
}

func TestParseObservationsErrors(t *testing.T) {
	loc := time.UTC
	for _, tc := range []struct {
		name    string
		csv     string
		wantErr string
	}{
		{
			name:    "empty input",
			csv:     "",
			wantErr: "input is empty",
		},
		{
			name:    "not an eBird export",
			csv:     "Name,Date\nAmerican Robin,2026-04-25\n",
			wantErr: "missing column(s) Common Name, Count",
		},
		{
			name:    "header only",
			csv:     sampleHeader,
			wantErr: "no observations found",
		},
		{
			name:    "empty common name",
			csv:     sampleHeader + "S1,,,1,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n",
			wantErr: "line 2: empty Common Name",
		},
		{
			name:    "empty date",
			csv:     sampleHeader + "S1,American Robin,Turdus migratorius,1,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,,,eBird - Casual Observation,,0,,,1\n",
			wantErr: "line 2: empty Date",
		},
		{
			name:    "bad date",
			csv:     sampleHeader + "S1,American Robin,Turdus migratorius,1,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,25 April 2026,,eBird - Casual Observation,,0,,,1\n",
			wantErr: `line 2: parsing Date "25 April 2026"`,
		},
		{
			name:    "bad time",
			csv:     sampleHeader + "S1,American Robin,Turdus migratorius,1,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,25:00,eBird - Casual Observation,,0,,,1\n",
			wantErr: `line 2: parsing Date/Time "2026-04-25" "25:00"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseObservations(strings.NewReader(tc.csv), &staticZoneFinder{loc: loc}, loc)
			if err == nil {
				t.Fatalf("got nil error, want one containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want one containing %q", err, tc.wantErr)
			}
		})
	}

	if _, err := parseObservations(strings.NewReader(sampleHeader), &staticZoneFinder{loc: loc}, loc); !errors.Is(err, errNoObservations) {
		t.Errorf("header-only input: got %v, want errNoObservations", err)
	}
}

func TestParseObservationsSkipsBlankLines(t *testing.T) {
	csv := sampleHeader +
		"S1,American Robin,Turdus migratorius,29740,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n" +
		"\n" +
		",,,,,,,,,,,,\n"

	if obs := mustParseObservations(t, csv, time.UTC); len(obs) != 1 {
		t.Errorf("got %d observations, want 1", len(obs))
	}
}

func TestParseObservationsToleratesBOMAndReorderedColumns(t *testing.T) {
	csv := "\ufeffCommon Name,Count,Date,Time,Submission ID\n" +
		"American Robin,3,2026-04-25,09:00 AM,S1\n"

	obs := mustParseObservations(t, csv, time.UTC)
	if len(obs) != 1 {
		t.Fatalf("got %d observations, want 1", len(obs))
	}
	if got, want := obs[0].Title(), "American Robin (3)"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	if got, want := obs[0].SubmissionID, "S1"; got != want {
		t.Errorf("SubmissionID = %q, want %q", got, want)
	}
}

func TestMostRecent(t *testing.T) {
	obs := []Observation{{CommonName: "a"}, {CommonName: "b"}, {CommonName: "c"}}
	if got := mostRecent(obs, 2); len(got) != 2 || got[1].CommonName != "b" {
		t.Errorf("mostRecent(obs, 2) = %+v", got)
	}
	if got := mostRecent(obs, 3); len(got) != 3 {
		t.Errorf("mostRecent(obs, 3) = %+v, want all 3", got)
	}
	if got := mostRecent(obs, 10); len(got) != 3 {
		t.Errorf("mostRecent(obs, 10) = %+v, want all 3", got)
	}
	if got := mostRecent(nil, 5); len(got) != 0 {
		t.Errorf("mostRecent(nil, 5) = %+v, want empty", got)
	}
}
