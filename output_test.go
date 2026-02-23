package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testResult() *InventoryResult {
	return &InventoryResult{
		Summary: InventorySummary{
			Total:    3,
			Orphaned: 1,
			ByType: map[string]int{
				"incoming_webhook": 2,
				"bot":              1,
			},
			ByCreatorStatus: map[string]int{
				"active":      2,
				"deactivated": 1,
			},
		},
		Integrations: []Integration{
			{
				Type:               TypeIncomingWebhook,
				ID:                 "iw1",
				Name:               "CI Webhook",
				CreatorUsername:     "alice",
				CreatorDisplayName: "Alice Johnson",
				CreatorStatus:      StatusActive,
				Team:               "Engineering",
				Channel:            "Dev Ops",
				Description:        "Build notifications",
				CreatedAt:          time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC),
				Orphaned:           false,
			},
			{
				Type:               TypeIncomingWebhook,
				ID:                 "iw2",
				Name:               "Alerts Webhook",
				CreatorUsername:     "bob.smith",
				CreatorDisplayName: "Bob Smith",
				CreatorStatus:      StatusDeactivated,
				Team:               "Engineering",
				Channel:            "Alerts",
				Description:        "Alert notifications",
				CreatedAt:          time.Date(2023, 11, 1, 9, 0, 0, 0, time.UTC),
				Orphaned:           true,
			},
			{
				Type:               TypeBot,
				ID:                 "bot1",
				Name:               "Deploy Bot",
				CreatorUsername:     "alice",
				CreatorDisplayName: "Alice Johnson",
				CreatorStatus:      StatusActive,
				Team:               "All Teams",
				Channel:            "",
				Description:        "Handles deploys",
				CreatedAt:          time.Date(2024, 1, 20, 14, 30, 0, 0, time.UTC),
				Orphaned:           false,
			},
		},
	}
}

func TestWriteCSV(t *testing.T) {
	result := testResult()
	var buf bytes.Buffer

	err := writeCSV(&buf, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}

	t.Run("header row", func(t *testing.T) {
		if len(records) < 1 {
			t.Fatal("no records")
		}
		expectedHeader := []string{
			"type", "name", "creator_username", "creator_display_name",
			"creator_status", "team", "channel", "description", "created_at", "orphaned",
		}
		for i, col := range expectedHeader {
			if records[0][i] != col {
				t.Errorf("header[%d] = %q, want %q", i, records[0][i], col)
			}
		}
	})

	t.Run("correct row count", func(t *testing.T) {
		if len(records) != 4 { // 1 header + 3 data
			t.Errorf("got %d rows, want 4 (1 header + 3 data)", len(records))
		}
	})

	t.Run("field values", func(t *testing.T) {
		row1 := records[1]
		if row1[0] != "incoming_webhook" {
			t.Errorf("type = %q, want %q", row1[0], "incoming_webhook")
		}
		if row1[1] != "CI Webhook" {
			t.Errorf("name = %q, want %q", row1[1], "CI Webhook")
		}
		if row1[9] != "false" {
			t.Errorf("orphaned = %q, want %q", row1[9], "false")
		}
	})

	t.Run("orphaned row has true", func(t *testing.T) {
		row2 := records[2]
		if row2[9] != "true" {
			t.Errorf("orphaned = %q, want %q", row2[9], "true")
		}
	})

	t.Run("date format is RFC3339", func(t *testing.T) {
		row1 := records[1]
		_, err := time.Parse("2006-01-02T15:04:05Z", row1[8])
		if err != nil {
			t.Errorf("date %q is not valid RFC3339: %v", row1[8], err)
		}
	})

	t.Run("empty channel is empty string", func(t *testing.T) {
		row3 := records[3] // bot
		if row3[6] != "" {
			t.Errorf("channel = %q, want empty string for bot", row3[6])
		}
	})
}

func TestWriteJSON(t *testing.T) {
	result := testResult()
	var buf bytes.Buffer

	err := writeJSON(&buf, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("valid JSON", func(t *testing.T) {
		if !json.Valid(buf.Bytes()) {
			t.Error("output is not valid JSON")
		}
	})

	t.Run("structure has summary and integrations", func(t *testing.T) {
		var parsed InventoryResult
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("JSON unmarshal error: %v", err)
		}
		if parsed.Summary.Total != 3 {
			t.Errorf("summary.total = %d, want 3", parsed.Summary.Total)
		}
		if len(parsed.Integrations) != 3 {
			t.Errorf("integrations count = %d, want 3", len(parsed.Integrations))
		}
	})

	t.Run("date format in JSON", func(t *testing.T) {
		var parsed InventoryResult
		json.Unmarshal(buf.Bytes(), &parsed)
		expected := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
		if !parsed.Integrations[0].CreatedAt.Equal(expected) {
			t.Errorf("created_at = %v, want %v", parsed.Integrations[0].CreatedAt, expected)
		}
	})
}

func TestWriteJSONEmptyList(t *testing.T) {
	result := &InventoryResult{
		Summary:      InventorySummary{Total: 0, ByType: map[string]int{}, ByCreatorStatus: map[string]int{}},
		Integrations: []Integration{},
	}
	var buf bytes.Buffer
	err := writeJSON(&buf, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid(buf.Bytes()) {
		t.Error("output is not valid JSON")
	}

	// Ensure integrations is [] not null
	if !strings.Contains(buf.String(), `"integrations": []`) {
		t.Error("empty integrations should be [] not null")
	}
}

func TestWriteTable(t *testing.T) {
	result := testResult()
	var buf bytes.Buffer

	err := writeTable(&buf, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	t.Run("type group headers present", func(t *testing.T) {
		if !strings.Contains(output, "=== Incoming Webhooks (2) ===") {
			t.Error("missing incoming webhooks group header")
		}
		if !strings.Contains(output, "=== Bot Accounts (1) ===") {
			t.Error("missing bot accounts group header")
		}
	})

	t.Run("orphan marker present", func(t *testing.T) {
		if !strings.Contains(output, "\u26a0") {
			t.Error("missing orphan warning marker")
		}
	})

	t.Run("summary section present", func(t *testing.T) {
		if !strings.Contains(output, "--- Summary ---") {
			t.Error("missing summary section")
		}
		if !strings.Contains(output, "Total integrations: 3") {
			t.Error("missing or incorrect total count")
		}
		if !strings.Contains(output, "Orphaned:           1") {
			t.Error("missing or incorrect orphan count")
		}
	})

	t.Run("N/A for empty channel", func(t *testing.T) {
		if !strings.Contains(output, "N/A") {
			t.Error("empty channel should show as N/A in table format")
		}
	})
}

func TestWriteTableEmpty(t *testing.T) {
	result := &InventoryResult{
		Summary:      InventorySummary{Total: 0, ByType: map[string]int{}, ByCreatorStatus: map[string]int{}},
		Integrations: []Integration{},
	}
	var buf bytes.Buffer
	err := writeTable(&buf, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "No integrations found.") {
		t.Error("expected 'No integrations found.' for empty result")
	}
}

func TestWriteTableOmitsEmptyTypes(t *testing.T) {
	result := &InventoryResult{
		Summary: InventorySummary{
			Total:    1,
			Orphaned: 0,
			ByType:   map[string]int{"bot": 1},
			ByCreatorStatus: map[string]int{"active": 1},
		},
		Integrations: []Integration{
			{
				Type: TypeBot, ID: "bot1", Name: "Test Bot",
				CreatorUsername: "alice", CreatorDisplayName: "Alice",
				CreatorStatus: StatusActive, Team: "All Teams",
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	var buf bytes.Buffer
	writeTable(&buf, result)
	output := buf.String()

	if strings.Contains(output, "Incoming Webhooks") {
		t.Error("empty incoming webhooks section should be omitted")
	}
	if strings.Contains(output, "Outgoing Webhooks") {
		t.Error("empty outgoing webhooks section should be omitted")
	}
	if !strings.Contains(output, "Bot Accounts") {
		t.Error("bot accounts section should be present")
	}
}

func TestWriteCSVEmptyList(t *testing.T) {
	result := &InventoryResult{
		Summary:      InventorySummary{Total: 0, ByType: map[string]int{}, ByCreatorStatus: map[string]int{}},
		Integrations: []Integration{},
	}
	var buf bytes.Buffer
	err := writeCSV(&buf, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("got %d rows, want 1 (header only)", len(records))
	}
}
