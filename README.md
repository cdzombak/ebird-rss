# ebird-rss

A small Go program that turns an [eBird](https://ebird.org) CSV export into an
RSS, Atom, or JSON feed of your most recent bird sightings.

Each item's title is the species' common name and count — "American Robin (3)",
or "American Robin (multiple)" when eBird recorded the species as present but
uncounted (`X`) — its description is where you saw it, and its date is the
observation's date and time, in the time zone of the place you were birding. Each
item links to the eBird checklist the sighting came from. The feed is written atomically, so a web server never serves
a half-written file.

The program reads only the export file on disk; it makes no network requests and
needs no eBird credentials.

## Getting your eBird data

Request an export at <https://ebird.org/downloadMyData>. eBird emails you a
`.zip` containing `MyEBirdData.csv`; that CSV is this program's input.

## Installation

### macOS via Homebrew

```shell
brew install cdzombak/oss/ebird-rss
```

### Debian via Apt repository

Install my Debian repository if you haven't already:

```shell
sudo apt-get install ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://dist.cdzombak.net/deb.key | sudo gpg --dearmor -o /etc/apt/keyrings/dist-cdzombak-net.gpg
sudo chmod 0644 /etc/apt/keyrings/dist-cdzombak-net.gpg
echo -e "deb [signed-by=/etc/apt/keyrings/dist-cdzombak-net.gpg] https://dist.cdzombak.net/deb/oss any oss\n" | sudo tee -a /etc/apt/sources.list.d/dist-cdzombak-net.list > /dev/null
sudo apt update
```

Then install `ebird-rss` via `apt`:

```shell
sudo apt install ebird-rss
```

### Manual installation from build artifacts

Pre-built binaries for Linux and macOS on multiple architectures are attached to every [GitHub Release](https://github.com/cdzombak/ebird-rss/releases). Debian packages for each release are published as well.

### Build and install locally

```shell
git clone https://github.com/cdzombak/ebird-rss.git
cd ebird-rss
make build

cp out/ebird-rss $INSTALL_DIR
```

### Docker image

Multi-architecture images are published to [Docker Hub](https://hub.docker.com/r/cdzombak/ebird-rss) and [GHCR](https://github.com/cdzombak/ebird-rss/pkgs/container/ebird-rss), built `FROM scratch` (just the binary). Both the IANA time zone database and the time zone boundary polygons are compiled into the binary, so zone lookup works in the container with no data files to mount.

Mount the directory holding your export and config, plus a writable directory for the feed:

```shell
docker run --rm \
  -v /home/cdzombak/ebird:/data \
  -v /var/www/feeds:/out \
  cdzombak/ebird-rss:1 \
  -in-file /data/MyEBirdData.csv \
  -config /data/config.yml \
  -out-file /out/birds.xml
```

## Configuration

The feed is described by a YAML file, passed with `-config`. A minimal example:

```yaml
count: 20
format: rss
fallback_timezone: "America/Detroit"
location_blocklist:
  - "Home"
feed:
  title: "Chris Dzombak • Bird Sightings"
  description: "Birds I've recently seen and logged to eBird."
  link: "https://ebird.org/profile/MTIzNDU2"
  feed_url: "https://www.dzombak.com/feeds/birds.rss.xml"
  author: "Chris Dzombak"
  language: "en-US"
```

Every field is optional and falls back to a default. See
[`config.example.yml`](config.example.yml) for the full, commented reference.

| Key                | Default                            | Description                                          |
| ------------------ | ---------------------------------- | ---------------------------------------------------- |
| `count`            | `20`                               | Number of sightings to include, most recent first.   |
| `format`           | `rss`                              | Output feed format: `rss`, `atom`, or `json`.        |
| `fallback_timezone` | the machine's local zone          | IANA time zone used only when a sighting's own zone can't be determined. |
| `location_blocklist` | empty                             | Location names to keep out of the feed; see [Hiding locations](#hiding-locations). |
| `feed.title`       | `eBird Sightings`                  | Feed title.                                          |
| `feed.description` | `Recent bird sightings from eBird.` | Feed description / subtitle.                         |
| `feed.link`        | `https://ebird.org/`               | The website the feed represents (home page).         |
| `feed.feed_url`    | —                                  | Canonical URL of the feed itself (`rel="self"`). Rendered in Atom and JSON output only, not RSS. |
| `feed.author`      | —                                  | Feed author.                                         |
| `feed.language`    | —                                  | Feed language, as a BCP 47 code (e.g. `en-US`).      |

Unknown keys are rejected, so a typo fails loudly instead of being ignored.

### About time zones

eBird's export records a local date and a local time of day with no zone or
offset. Rather than assume every sighting shares one zone, the program resolves
each observation's coordinates — which the export includes on every row — to the
time zone in effect there, so a checklist from a trip is dated correctly relative
to one from home. Michigan and Florida each span two zones, so this matters even
without leaving your state.

The lookup is entirely offline: the
[timezone-boundary-builder](https://github.com/evansiroky/timezone-boundary-builder)
polygons are compiled into the binary via
[`ringsaturn/tzf`](https://github.com/ringsaturn/tzf), which is what makes the
binary ~33 MB. Loading them costs about 300ms once per run, and each distinct
location is looked up only once.

`fallback_timezone` covers the rows that lookup can't place: a row with no
coordinates, or coordinates that land nowhere. When any sighting falls back, the
program says so on stderr. It defaults to the machine's local zone; set it
explicitly when generating the feed on a server or in a container, where "local"
is usually UTC.

Checklists submitted without a time of day (eBird's "casual observation"
protocol, among others) are dated to midnight local time on the day observed.

### Hiding locations

Each item's description is where the sighting happened, as
`Location, County, State/Province` — for example
`Gallup Park, Washtenaw, US-MI`.

`Location` is whatever the site is named in your eBird account, and an eBird
"personal location" is named by you: it's often `Home`, and it can be a full
street address. `location_blocklist` is a list of strings; if any of them appears
in a location's name, that name is dropped from the feed and the description is
just `County, State/Province`.

```yaml
location_blocklist:
  - "Home"
  - "Sparrow Lane"
  - "1234"
```

Matching is on substrings and ignores case, so `sparrow lane` also hides
`1234 Sparrow Lane, Anytown`. An empty entry is rejected, since it would match
every location and silently hide them all.

**This only controls what this feed publishes.** Items still link to the eBird
checklist, and that page is public — check what eBird itself shows for your
personal locations before publishing a feed that includes them.

## Usage

```sh
ebird-rss -in-file MyEBirdData.csv -config config.yml -out-file birds.xml
```

`-out-file -` writes the feed to stdout instead of a file:

```sh
ebird-rss -in-file MyEBirdData.csv -config config.yml -out-file -
```

### Flags

`-help` prints usage and exits; `-version` prints the version and exits.

| Flag        | Required | Description                                                               |
| ----------- | -------- | ------------------------------------------------------------------------- |
| `-in-file`  | yes      | Path to the eBird CSV export (`MyEBirdData.csv`) to read.                 |
| `-out-file` | yes      | Path to write the output feed to (written atomically), or `-` for stdout. |
| `-config`   | yes      | Path to the YAML [feed configuration](#configuration).                    |
| `-verbose`  | no       | Enable verbose (debug) logging to stderr.                                 |

The sighting count, output format, and all feed metadata live in the
[config file](#configuration).

## Building from source

```sh
make build          # build for the current platform to ./out
make all            # cross-compile for macOS and Linux (amd64/arm64/armv7/armv6)
make package        # build binaries + .deb packages (requires fpm)
make test           # run the test suite
make lint           # lint (requires golangci-lint)
```

The build stamps the version (from `.version.sh`) into the binary, reported by
`-version`. `go run .` and un-stamped builds report `<dev>`.

## License

GPL-3.0; see [LICENSE](LICENSE).
