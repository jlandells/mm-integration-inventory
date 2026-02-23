package main

import (
	"fmt"
	"testing"
)

func TestExitCodeForError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"nil error returns success", nil, ExitSuccess},
		{"ConfigError returns 1", &ConfigError{Message: "missing url"}, ExitConfigError},
		{"APIError returns 2", &APIError{Message: "server error", StatusCode: 500}, ExitAPIError},
		{"PartialError returns 3", &PartialError{Message: "some failed", FailCount: 2, TotalCount: 10}, ExitPartialFailure},
		{"OutputError returns 4", &OutputError{Message: "write failed"}, ExitOutputError},
		{"unknown error returns 2 (APIError)", fmt.Errorf("some other error"), ExitAPIError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exitCodeForError(tt.err)
			if got != tt.wantCode {
				t.Errorf("exitCodeForError(%v) = %d, want %d", tt.err, got, tt.wantCode)
			}
		})
	}
}

func TestConfigErrorMessage(t *testing.T) {
	e := &ConfigError{Message: "server URL is required"}
	if e.Error() != "server URL is required" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestAPIErrorMessage(t *testing.T) {
	t.Run("with status code", func(t *testing.T) {
		e := &APIError{Message: "server error", StatusCode: 500}
		expected := "server error (HTTP 500)"
		if e.Error() != expected {
			t.Errorf("Error() = %q, want %q", e.Error(), expected)
		}
	})

	t.Run("without status code", func(t *testing.T) {
		e := &APIError{Message: "connection failed"}
		if e.Error() != "connection failed" {
			t.Errorf("Error() = %q", e.Error())
		}
	})
}

func TestPartialErrorMessage(t *testing.T) {
	e := &PartialError{Message: "lookup failed", FailCount: 3, TotalCount: 10}
	expected := "lookup failed (3 of 10 failed)"
	if e.Error() != expected {
		t.Errorf("Error() = %q, want %q", e.Error(), expected)
	}
}

func TestOutputErrorMessage(t *testing.T) {
	e := &OutputError{Message: "unable to write file"}
	if e.Error() != "unable to write file" {
		t.Errorf("Error() = %q", e.Error())
	}
}
