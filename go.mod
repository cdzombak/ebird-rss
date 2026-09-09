module github.com/cdzombak/ebird-rss

go 1.26

require (
	github.com/cdzombak/exitcode_go v1.0.0
	github.com/mmcdole/gofeed v1.3.0
	github.com/ringsaturn/tzf v1.2.3
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/PuerkitoBio/goquery v1.8.0 // indirect
	github.com/andybalholm/cascadia v1.3.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mmcdole/goxpp v1.1.1-0.20240225020742-a0c311522b23 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/paulmach/orb v0.13.0 // indirect
	github.com/ringsaturn/tzf-dist v0.0.2026-c-fix1 // indirect
	github.com/tidwall/geoindex v1.7.0 // indirect
	github.com/tidwall/rtree v1.10.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// Use cdzombak's fork (cdz/feed-creation branch), which adds feed generation
// (Feed.RenderRSS/RenderAtom/RenderJSON). The fork keeps the upstream module path.
replace github.com/mmcdole/gofeed => github.com/cdzombak/gofeed v0.0.0-20250914230300-21507eb34063
