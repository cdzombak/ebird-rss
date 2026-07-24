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

	if got, want := public.Description(nil), "Lincoln Twp. Park, Berrien, US-MI"; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
	if got, want := public.Description(bl), "Lincoln Twp. Park, Berrien, US-MI"; got != want {
		t.Errorf("unmatched location should survive the blocklist: %q, want %q", got, want)
	}
	if got, want := private.Description(bl), "Washtenaw, US-MI"; got != want {
		t.Errorf("blocked location: Description() = %q, want %q", got, want)
	}
	// Without the blocklist, that same sighting publishes the street address.
	if got, want := private.Description(nil), "1234 Sparrow Lane, Anytown, MI 99999, Washtenaw, US-MI"; got != want {
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
		{"no county", Observation{Location: "Gallup Park", StateProvince: "US-MI"}, "Gallup Park, US-MI"},
		{"no state", Observation{Location: "Gallup Park", County: "Washtenaw"}, "Gallup Park, Washtenaw"},
		{"location only", Observation{Location: "Gallup Park"}, "Gallup Park"},
		{"county and state only", Observation{County: "Washtenaw", StateProvince: "US-MI"}, "Washtenaw, US-MI"},
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
