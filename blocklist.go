package main

import "strings"

// locationBlocklist hides eBird location names you don't want published.
//
// An eBird "personal location" is named by whoever created it, so the export's
// Location column can carry a street address, a house number, or the name of a
// road you live on. Any location whose name contains one of these strings is
// omitted from the feed, leaving only the county and state.
type locationBlocklist []string

// hides reports whether location matches any entry. Matching is on substrings
// and ignores case, because the point is to catch a name however it was typed:
// "sparrow lane" should hide "1234 Sparrow Lane, Anytown".
func (b locationBlocklist) hides(location string) bool {
	if len(b) == 0 {
		return false
	}
	folded := strings.ToLower(location)
	for _, entry := range b {
		if strings.Contains(folded, strings.ToLower(entry)) {
			return true
		}
	}
	return false
}
