package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

// eBird's CSV export ("MyEBirdData.csv") splits an observation's timestamp
// across two columns: a date, and a time that is empty for checklists submitted
// without one (e.g. incidental observations).
const (
	ebirdDateLayout     = "2006-01-02"
	ebirdDateTimeLayout = "2006-01-02 03:04 PM"
)

// noonHour dates a checklist that carries no time of day. Midday puts it among
// that day's timed sightings; midnight would make it the oldest instant of the
// day, so a reader would file it below every sighting it was recorded alongside.
const noonHour = 12

// Columns read from the export; the rest (protocol, duration, checklist
// comments, …) are ignored. Only requiredColumns must be present, so an export
// that gains or loses other columns still parses.
const (
	colSubmissionID       = "Submission ID"
	colCommonName         = "Common Name"
	colScientificName     = "Scientific Name"
	colCount              = "Count"
	colLocation           = "Location"
	colCounty             = "County"
	colStateProvince      = "State/Province"
	colLatitude           = "Latitude"
	colLongitude          = "Longitude"
	colDate               = "Date"
	colTime               = "Time"
	colBreedingCode       = "Breeding Code"
	colObservationDetails = "Observation Details"
)

// requiredColumns must all be present in the export's header row.
var requiredColumns = []string{colCommonName, colCount, colDate}

// errNoObservations reports an export whose header row is fine but which
// contains no observations.
var errNoObservations = errors.New("no observations found")

// Observation is one row of the eBird export: a single species seen on a single
// checklist.
type Observation struct {
	SubmissionID   string
	CommonName     string
	ScientificName string
	// Count is the raw eBird value: a number, or "X" for "present, but not
	// counted". It may be empty if the export omits it.
	Count string
	// Location, County, and StateProvince describe where the sighting happened.
	// Location is whatever the observer named the site, which for a personal
	// location can be a home address; see locationBlocklist.
	Location      string
	County        string
	StateProvince string
	// BreedingCode is eBird's breeding-behavior code and its label, as the export
	// writes them together — "S Singing Bird".
	BreedingCode string
	// Details is the observer's free-text note about this species on this
	// checklist.
	Details string
	// ObservedAt is the checklist's date and time, in the time zone of the place
	// it was recorded. When the export carries no time of day, it is noon there;
	// see noonHour.
	ObservedAt time.Time
	// ZoneFallback records that the observation's coordinates couldn't be
	// resolved to a time zone, so the configured fallback was used instead. The
	// caller reports how many rows this happened to.
	ZoneFallback bool
}

// parseObservations reads an eBird "MyEBirdData.csv" export and returns its
// observations sorted newest first.
//
// The export records neither an offset nor a zone, so each row's date and time
// are interpreted in the zone the finder resolves its coordinates to; rows
// without usable coordinates fall back to fallbackLoc.
//
// Sorting compares absolute instants, so observations from different zones
// interleave correctly. Ties (every species on one checklist shares its
// timestamp) are broken by submission ID then common name, so the output is
// stable across runs.
func parseObservations(r io.Reader, finder zoneFinder, fallbackLoc *time.Location) ([]Observation, error) {
	cr := csv.NewReader(r)
	// Rows vary in length: trailing empty columns are sometimes omitted.
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("input is empty; expected an eBird CSV export")
		}
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	cols, err := indexColumns(header)
	if err != nil {
		return nil, err
	}

	var obs []Observation
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading CSV: %w", err)
		}
		if isBlankRecord(rec) {
			continue
		}
		// csv reports the line just read; the header occupies line 1.
		line, _ := cr.FieldPos(0)
		o, err := observationFromRecord(rec, cols, finder, fallbackLoc)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		obs = append(obs, o)
	}

	if len(obs) == 0 {
		return nil, errNoObservations
	}

	slices.SortStableFunc(obs, func(a, b Observation) int {
		if c := b.ObservedAt.Compare(a.ObservedAt); c != 0 { // b first: newest first
			return c
		}
		if c := strings.Compare(a.SubmissionID, b.SubmissionID); c != 0 {
			return c
		}
		return strings.Compare(a.CommonName, b.CommonName)
	})
	return obs, nil
}

// indexColumns maps the column names we care about to their positions in the
// header row, and verifies that the required ones are present.
func indexColumns(header []string) (map[string]int, error) {
	cols := make(map[string]int, len(header))
	for i, name := range header {
		// The first cell of a UTF-8 export may carry a byte-order mark.
		cols[strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))] = i
	}
	var missing []string
	for _, name := range requiredColumns {
		if _, ok := cols[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("input does not look like an eBird CSV export: missing column(s) %s",
			strings.Join(missing, ", "))
	}
	return cols, nil
}

// observationFromRecord builds an Observation from one CSV row.
func observationFromRecord(rec []string, cols map[string]int, finder zoneFinder, fallbackLoc *time.Location) (Observation, error) {
	field := func(name string) string {
		i, ok := cols[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	o := Observation{
		SubmissionID:   field(colSubmissionID),
		CommonName:     field(colCommonName),
		ScientificName: field(colScientificName),
		Count:          field(colCount),
		Location:       field(colLocation),
		County:         field(colCounty),
		StateProvince:  field(colStateProvince),
		BreedingCode:   field(colBreedingCode),
		Details:        field(colObservationDetails),
	}
	if o.CommonName == "" {
		return Observation{}, errors.New("empty Common Name")
	}

	loc, fellBack, err := recordZone(field(colLatitude), field(colLongitude), finder, fallbackLoc)
	if err != nil {
		return Observation{}, err
	}
	o.ZoneFallback = fellBack

	date, timeOfDay := field(colDate), field(colTime)
	if date == "" {
		return Observation{}, errors.New("empty Date")
	}
	if timeOfDay == "" {
		t, err := time.ParseInLocation(ebirdDateLayout, date, loc)
		if err != nil {
			return Observation{}, fmt.Errorf("parsing Date %q: %w", date, err)
		}
		// Constructed rather than added to the parsed midnight, so a day that
		// gains or loses an hour to DST still lands at noon.
		o.ObservedAt = time.Date(t.Year(), t.Month(), t.Day(), noonHour, 0, 0, 0, loc)
		return o, nil
	}

	t, err := time.ParseInLocation(ebirdDateTimeLayout, date+" "+timeOfDay, loc)
	if err != nil {
		return Observation{}, fmt.Errorf("parsing Date/Time %q %q: %w", date, timeOfDay, err)
	}
	o.ObservedAt = t
	return o, nil
}

// recordZone resolves one row's coordinates to a time zone, reporting whether
// it had to fall back.
//
// Coordinates that are absent entirely fall back quietly — some exports omit
// them. Coordinates that are present but unreadable are an error, because that
// means the file isn't shaped the way this program believes it is.
//
// A lookup that fails falls back only when the finder reports that nothing
// covers the coordinate. Any other failure means the finder itself is broken —
// boundary data that wouldn't load, say — and falling back on that would date
// every row in the fallback zone and publish it as if it were right.
func recordZone(lat, lon string, finder zoneFinder, fallbackLoc *time.Location) (loc *time.Location, fellBack bool, err error) {
	if lat == "" || lon == "" {
		return fallbackLoc, true, nil
	}
	latF, err := strconv.ParseFloat(lat, 64)
	if err != nil {
		return nil, false, fmt.Errorf("parsing Latitude %q: %w", lat, err)
	}
	lonF, err := strconv.ParseFloat(lon, 64)
	if err != nil {
		return nil, false, fmt.Errorf("parsing Longitude %q: %w", lon, err)
	}
	zone, err := finder.zoneAt(latF, lonF)
	if errors.Is(err, errNoZoneForCoords) {
		// Not worth failing the whole run over; the caller reports how many
		// rows this happened to.
		return fallbackLoc, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return zone, false, nil
}

// isBlankRecord reports whether a row has no content, as trailing blank lines
// in an export otherwise parse as an observation with no name.
func isBlankRecord(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}

// mostRecent returns the first n observations, which are expected to be sorted
// newest first. A shorter list is returned unchanged.
func mostRecent(obs []Observation, n int) []Observation {
	return obs[:min(n, len(obs))]
}
