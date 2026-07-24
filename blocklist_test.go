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
