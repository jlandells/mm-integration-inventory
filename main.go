package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	// Standard flags
	url := flag.String("url", envOrDefault("MM_URL", ""), "Mattermost server URL")
	token := flag.String("token", envOrDefault("MM_TOKEN", ""), "Personal Access Token")
	username := flag.String("username", envOrDefault("MM_USERNAME", ""), "Username for password auth")
	format := flag.String("format", "table", "Output format: table, csv, json")
	output := flag.String("output", "", "Write output to this file path")
	verbose := flag.Bool("verbose", false, "Enable verbose logging to stderr")
	flag.BoolVar(verbose, "v", false, "Enable verbose logging to stderr (shorthand)")
	showVersion := flag.Bool("version", false, "Print version and exit")

	// Tool-specific flags
	team := flag.String("team", "", "Scope report to a single named team")
	orphanedOnly := flag.Bool("orphaned-only", false, "Show only orphaned integrations")
	typeFilter := flag.String("type", "", "Filter by type: incoming, outgoing, bot, oauth, slash")

	flag.Parse()

	if *showVersion {
		fmt.Println("mm-integration-inventory " + version)
		return ExitSuccess
	}

	// Validate required flags
	if *url == "" {
		fmt.Fprintln(os.Stderr, "error: server URL is required. Use --url or set the MM_URL environment variable.")
		return ExitConfigError
	}

	// Validate format
	switch *format {
	case "table", "csv", "json":
		// valid
	default:
		fmt.Fprintf(os.Stderr, "error: invalid format %q. Valid formats: table, csv, json\n", *format)
		return ExitConfigError
	}

	// Validate type filter
	var parsedType IntegrationType
	if *typeFilter != "" {
		var err error
		parsedType, err = ParseIntegrationType(*typeFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return ExitConfigError
		}
	}

	// Authenticate and create client
	client, err := newClient(*url, *token, *username, *verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return exitCodeForError(err)
	}

	if *verbose {
		fmt.Fprintln(os.Stderr, "Connected successfully.")
	}

	// Fetch inventory
	fetcher := NewInventoryFetcher(client, *verbose)
	result, err := fetcher.FetchInventory(*team, parsedType, *orphanedOnly)
	if err != nil {
		// If we got a partial result, still output it
		if result != nil {
			if outputErr := writeOutput(result, *format, *output); outputErr != nil {
				fmt.Fprintf(os.Stderr, "%v\n", outputErr)
				return ExitOutputError
			}
		}
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return exitCodeForError(err)
	}

	// Write output
	if err := writeOutput(result, *format, *output); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return ExitOutputError
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Done. %d integrations found.\n", result.Summary.Total)
	}

	return ExitSuccess
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
