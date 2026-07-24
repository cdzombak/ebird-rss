package main

import "testing"

// Every location in these tests is invented. Real personal locations don't
// belong in a public repo — which is the whole point of the blocklist.
func TestLocationBlocklistHides(t *testing.T) {
	bl := locationBlocklist{"Home", "Sparrow Lane", "9999"}

	for _, tc := range []struct {
		location string
		want     bool
	}{
		{"Home", true},
		{"Home feeders", true},
		{"1234 Sparrow Lane, Anytown", true},
		{"9999 Example Street", true},
		// Case-insensitive, since the point is to catch a name however it was
		// typed into eBird.
		{"home", true},
		{"SPARROW LANE", true},
		{"my home office window", true},
		// Public places stay.
		{"Grand Mere State Park", false},
		{"Lincoln Twp. Park", false},
		{"Gallup Park", false},
		{"", false},
	} {
		if got := bl.hides(tc.location); got != tc.want {
			t.Errorf("hides(%q) = %v, want %v", tc.location, got, tc.want)
		}
	}
}

func TestLocationBlocklistEmpty(t *testing.T) {
	var bl locationBlocklist
	if bl.hides("Home") {
		t.Error("an empty blocklist should hide nothing")
	}
	if (locationBlocklist{}).hides("Home") {
		t.Error("an empty blocklist should hide nothing")
	}
}

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
