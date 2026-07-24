// Command ebird-rss turns an eBird CSV export ("MyEBirdData.csv", from
// https://ebird.org/downloadMyData) into a feed of your most recent sightings.
//
// Each item's title is the species' common name and count — "American Robin
// (3)", or "American Robin (multiple)" when eBird recorded the species as
// present but uncounted — and its date is the observation's date and time. The
// number of sightings, the output format, and the feed's metadata are read from
// a YAML file given with -config.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Embed the IANA time zone database, so the config's `timezone` works even
	// where the system has no zoneinfo (notably the FROM scratch Docker image).
	_ "time/tzdata"

	exitcode "github.com/cdzombak/exitcode_go"
)

// appName is the program name, used in the feed's generator field.
const appName = "ebird-rss"

// version is the program version, injected at build time via
// -ldflags="-X main.version=...". It defaults to a placeholder for `go run`
// and un-stamped builds.
var version = "<dev>"

// cliArgs holds the parsed command-line configuration.
type cliArgs struct {
	inFile     string
	outFile    string
	configPath string
	verbose    bool
}

func main() {
	var args cliArgs
	var showHelp, showVersion bool
	flag.BoolVar(&showHelp, "help", false, "Show this help and exit.")
	flag.BoolVar(&showVersion, "version", false, "Print the version and exit.")
	flag.StringVar(&args.inFile, "in-file", "", "Path to the eBird CSV export (MyEBirdData.csv) to read. Required.")
	flag.StringVar(&args.outFile, "out-file", "", `Path to write the output feed to, or "-" for stdout. Required.`)
	flag.StringVar(&args.configPath, "config", "", "Path to the YAML feed configuration file. Required. See config.example.yml.")
	flag.BoolVar(&args.verbose, "verbose", false, "Enable verbose (debug) logging to stderr.")
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		os.Exit(exitcode.Success)
	}

	if showHelp {
		flag.CommandLine.SetOutput(os.Stdout)
		flag.Usage()
		os.Exit(exitcode.Success)
	}

	if err := validateArgs(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		flag.Usage()
		os.Exit(exitcode.InvalidArgument)
	}

	logLevel := slog.LevelWarn
	if args.verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	if err := run(args, logger); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitcode.Failure)
	}
}

// validateArgs checks the argument combination. Errors here are usage errors.
func validateArgs(args cliArgs) error {
	if args.inFile == "" {
		return errors.New("-in-file is required")
	}
	if args.outFile == "" {
		return errors.New("-out-file is required")
	}
	if args.configPath == "" {
		return errors.New("-config is required")
	}
	return nil
}

func run(args cliArgs, logger *slog.Logger) error {
	fc, err := loadConfig(args.configPath)
	if err != nil {
		return err
	}
	return generateFeed(args.inFile, args.outFile, fc, time.Now(), logger)
}

// generateFeed reads the export at inFile and writes the configured feed of its
// most recent observations to outFile. now is used as the feed's update time
// only when no observation supplies one.
func generateFeed(inFile, outFile string, fc feedConfig, now time.Time, logger *slog.Logger) error {
	f, err := os.Open(inFile)
	if err != nil {
		return fmt.Errorf("open eBird export %q: %w", inFile, err)
	}
	defer func() { _ = f.Close() }()

	obs, err := parseObservations(f, fc.Location())
	if err != nil {
		return fmt.Errorf("reading eBird export %q: %w", inFile, err)
	}
	logger.Debug("parsed eBird export", "path", inFile, "observations", len(obs))

	feed := buildFeed(mostRecent(obs, fc.Count), fc, now)
	if err := writeFeed(feed, fc.Format, outFile); err != nil {
		return err
	}

	dest := outFile
	if dest == "-" {
		dest = "stdout"
	}
	logger.Debug("wrote feed", "format", fc.Format, "items", len(feed.Items), "dest", dest)
	return nil
}
