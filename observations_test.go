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

func mustParseObservations(t *testing.T, csv string, loc *time.Location) []Observation {
	t.Helper()
	obs, err := parseObservations(strings.NewReader(csv), loc)
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
	obs, err := parseObservations(f, loc)
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
	if !first.HasTime {
		t.Error("obs[0].HasTime = false, want true")
	}
	if first.Location != "Grand Mere State Park, Stevensville US-MI 42.00341, -86.54192" {
		t.Errorf("quoted location not parsed: %q", first.Location)
	}
	if got, want := first.ChecklistURL(), "https://ebird.org/checklist/S327776301"; got != want {
		t.Errorf("ChecklistURL() = %q, want %q", got, want)
	}
	if got, want := first.GUID(), "ebird:S327776301:Turdus migratorius"; got != want {
		t.Errorf("GUID() = %q, want %q", got, want)
	}

	// The Wood Duck row has no Time and a short record (trailing columns
	// omitted); it lands at midnight local.
	last := obs[len(obs)-1]
	if want := time.Date(2023, 10, 7, 0, 0, 0, 0, loc); !last.ObservedAt.Equal(want) {
		t.Errorf("timeless observation = %s, want %s", last.ObservedAt, want)
	}
	if last.HasTime {
		t.Error("timeless observation: HasTime = true, want false")
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

func TestParseObservationsUsesConfiguredLocation(t *testing.T) {
	csv := sampleHeader +
		"S1,American Robin,Turdus migratorius,29740,1,US-MI,Berrien,L1,Lincoln Twp. Park,0,0,2026-04-25,09:00 AM,eBird - Casual Observation,,0,,,1\n"

	detroit := mustParseObservations(t, csv, mustLocation(t, "America/Detroit"))[0]
	utc := mustParseObservations(t, csv, time.UTC)[0]

	if detroit.ObservedAt.Equal(utc.ObservedAt) {
		t.Errorf("same wall time in different zones should differ: %s vs %s", detroit.ObservedAt, utc.ObservedAt)
	}
	if got := detroit.ObservedAt.UTC().Hour(); got != 13 { // 09:00 EDT == 13:00 UTC
		t.Errorf("09:00 America/Detroit = %02d:00 UTC, want 13:00", got)
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
			_, err := parseObservations(strings.NewReader(tc.csv), loc)
			if err == nil {
				t.Fatalf("got nil error, want one containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want one containing %q", err, tc.wantErr)
			}
		})
	}

	if _, err := parseObservations(strings.NewReader(sampleHeader), loc); !errors.Is(err, errNoObservations) {
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
