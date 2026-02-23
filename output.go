package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode"
	"unicode/utf8"
)

// writeOutput dispatches to the appropriate formatter and handles file output.
func writeOutput(result *InventoryResult, format, outputPath string) error {
	var w io.Writer = os.Stdout

	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: unable to write to %s: %v — writing to stdout instead.\n", outputPath, err)
		} else {
			defer f.Close()
			w = f
		}
	}

	switch strings.ToLower(format) {
	case "csv":
		return writeCSV(w, result)
	case "json":
		return writeJSON(w, result)
	default:
		return writeTable(w, result)
	}
}

// --- Table formatter ---

func writeTable(w io.Writer, result *InventoryResult) error {
	if len(result.Integrations) == 0 {
		fmt.Fprintln(w, "No integrations found.")
		return nil
	}

	// Group integrations by type in display order.
	grouped := make(map[IntegrationType][]Integration)
	for _, ig := range result.Integrations {
		grouped[ig.Type] = append(grouped[ig.Type], ig)
	}

	for _, t := range AllTypes {
		items, ok := grouped[t]
		if !ok || len(items) == 0 {
			continue
		}

		fmt.Fprintf(w, "\n=== %s (%d) ===\n\n", TypeDisplayName(t), len(items))

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tCREATOR\tCREATOR STATUS\tTEAM\tCHANNEL\tCREATED")
		for _, ig := range items {
			statusStr := capitalise(string(ig.CreatorStatus))
			if ig.Orphaned {
				statusStr += " \u26a0"
			}

			channel := ig.Channel
			if channel == "" {
				channel = "N/A"
			}

			createdStr := ""
			if !ig.CreatedAt.IsZero() {
				createdStr = ig.CreatedAt.Format("2006-01-02")
			}

			name := ig.Name
			if len(name) > 30 {
				name = name[:27] + "..."
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				name, ig.CreatorUsername, statusStr, ig.Team, channel, createdStr)
		}
		tw.Flush()
	}

	// Summary section
	fmt.Fprintln(w)
	fmt.Fprintln(w, "--- Summary ---")
	fmt.Fprintf(w, "Total integrations: %d\n", result.Summary.Total)
	fmt.Fprintf(w, "Orphaned:           %d\n", result.Summary.Orphaned)

	if len(result.Summary.ByType) > 0 {
		fmt.Fprintln(w, "\nBy type:")
		for _, t := range AllTypes {
			if count, ok := result.Summary.ByType[string(t)]; ok {
				fmt.Fprintf(w, "  %-22s %d\n", TypeDisplayName(t)+":", count)
			}
		}
	}

	if result.Summary.TeamFilter != "" {
		fmt.Fprintf(w, "\nFiltered to team: %s\n", result.Summary.TeamFilter)
	}
	if result.Summary.TypeFilter != "" {
		fmt.Fprintf(w, "Filtered to type: %s\n", result.Summary.TypeFilter)
	}
	if result.Summary.OrphanedOnlyFilter {
		fmt.Fprintln(w, "Showing orphaned integrations only.")
	}

	return nil
}

// --- CSV formatter ---

func writeCSV(w io.Writer, result *InventoryResult) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{
		"type", "name", "creator_username", "creator_display_name",
		"creator_status", "team", "channel", "description", "created_at", "orphaned",
	}
	if err := cw.Write(header); err != nil {
		return &OutputError{Message: fmt.Sprintf("error writing CSV header: %v", err)}
	}

	for _, ig := range result.Integrations {
		createdAt := ""
		if !ig.CreatedAt.IsZero() {
			createdAt = ig.CreatedAt.Format("2006-01-02T15:04:05Z")
		}

		orphaned := "false"
		if ig.Orphaned {
			orphaned = "true"
		}

		row := []string{
			string(ig.Type),
			ig.Name,
			ig.CreatorUsername,
			ig.CreatorDisplayName,
			string(ig.CreatorStatus),
			ig.Team,
			ig.Channel,
			ig.Description,
			createdAt,
			orphaned,
		}
		if err := cw.Write(row); err != nil {
			return &OutputError{Message: fmt.Sprintf("error writing CSV row: %v", err)}
		}
	}
	return nil
}

// --- JSON formatter ---

func writeJSON(w io.Writer, result *InventoryResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return &OutputError{Message: fmt.Sprintf("error writing JSON: %v", err)}
	}
	return nil
}

// capitalise returns s with the first letter upper-cased.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
