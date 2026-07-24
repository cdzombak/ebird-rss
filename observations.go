package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
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

// countMultiple is the Count value eBird uses for "present, but not counted".
const countMultiple = "X"

// multipleLabel is how countMultiple is rendered in an item title.
const multipleLabel = "multiple"

// checklistURLPrefix is the public eBird URL for a checklist, by submission ID.
const checklistURLPrefix = "https://ebird.org/checklist/"

// Columns read from the export. Others (coordinates, protocol, breeding code,
// …) are ignored. Only colCommonName, colCount, and colDate are required, so an
// export that gains or loses other columns still parses.
const (
	colSubmissionID   = "Submission ID"
	colCommonName     = "Common Name"
	colScientificName = "Scientific Name"
	colCount          = "Count"
	colLocation       = "Location"
	colDate           = "Date"
	colTime           = "Time"
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
	Count    string
	Location string
	// ObservedAt is the checklist's date and time, in the configured location.
	// When the export carries no time, it is midnight and HasTime is false.
	ObservedAt time.Time
	HasTime    bool
}

// Title is the observation's feed item title: the common name followed by the
// count in parentheses. An uncounted ("X") or missing count reads as
// "multiple".
func (o Observation) Title() string {
	return fmt.Sprintf("%s (%s)", o.CommonName, o.CountLabel())
}

// CountLabel renders the raw eBird count for display.
func (o Observation) CountLabel() string {
	c := strings.TrimSpace(o.Count)
	if c == "" || strings.EqualFold(c, countMultiple) {
		return multipleLabel
	}
	return c
}

// ChecklistURL is the public eBird page for the checklist this observation came
// from, or "" if the export carried no submission ID.
func (o Observation) ChecklistURL() string {
	if o.SubmissionID == "" {
		return ""
	}
	return checklistURLPrefix + o.SubmissionID
}

// GUID is a stable, unique identifier for the observation. It is not a URL: a
// checklist's URL is shared by every species on that checklist, so the species
// has to be part of the identity.
func (o Observation) GUID() string {
	name := o.ScientificName
	if name == "" {
		name = o.CommonName
	}
	return fmt.Sprintf("ebird:%s:%s", o.SubmissionID, name)
}

// parseObservations reads an eBird "MyEBirdData.csv" export and returns its
// observations sorted newest first. Dates and times are interpreted in loc,
// since the export records neither an offset nor a zone.
//
// Ties (every species on one checklist shares its timestamp) are broken by
// submission ID then common name, so the output is stable across runs.
func parseObservations(r io.Reader, loc *time.Location) ([]Observation, error) {
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
		o, err := observationFromRecord(rec, cols, loc)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		obs = append(obs, o)
	}

	if len(obs) == 0 {
		return nil, errNoObservations
	}

	sort.SliceStable(obs, func(i, j int) bool {
		a, b := obs[i], obs[j]
		if !a.ObservedAt.Equal(b.ObservedAt) {
			return a.ObservedAt.After(b.ObservedAt)
		}
		if a.SubmissionID != b.SubmissionID {
			return a.SubmissionID < b.SubmissionID
		}
		return a.CommonName < b.CommonName
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
func observationFromRecord(rec []string, cols map[string]int, loc *time.Location) (Observation, error) {
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
	}
	if o.CommonName == "" {
		return Observation{}, errors.New("empty Common Name")
	}

	date, timeOfDay := field(colDate), field(colTime)
	if date == "" {
		return Observation{}, errors.New("empty Date")
	}
	if timeOfDay == "" {
		t, err := time.ParseInLocation(ebirdDateLayout, date, loc)
		if err != nil {
			return Observation{}, fmt.Errorf("parsing Date %q: %w", date, err)
		}
		o.ObservedAt = t
		return o, nil
	}

	t, err := time.ParseInLocation(ebirdDateTimeLayout, date+" "+timeOfDay, loc)
	if err != nil {
		return Observation{}, fmt.Errorf("parsing Date/Time %q %q: %w", date, timeOfDay, err)
	}
	o.ObservedAt = t
	o.HasTime = true
	return o, nil
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
	if n < len(obs) {
		return obs[:n]
	}
	return obs
}
