package main

import "fmt"

// Exit codes — every non-zero code has a specific meaning for scripting.
const (
	ExitSuccess        = 0 // Successful run
	ExitConfigError    = 1 // Missing flags, invalid input, auth failure, unknown team
	ExitAPIError       = 2 // Connection failure, unexpected API response
	ExitPartialFailure = 3 // Operation completed but with some per-item lookup failures
	ExitOutputError    = 4 // Unable to write output file
)

// ConfigError represents a configuration or validation problem.
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// APIError represents a failure communicating with the Mattermost API.
type APIError struct {
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
	}
	return e.Message
}

// PartialError indicates the run completed but some items encountered errors.
type PartialError struct {
	Message    string
	FailCount  int
	TotalCount int
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("%s (%d of %d failed)", e.Message, e.FailCount, e.TotalCount)
}

// OutputError represents a failure writing output.
type OutputError struct {
	Message string
}

func (e *OutputError) Error() string {
	return e.Message
}

// exitCodeForError maps an error type to the appropriate exit code.
func exitCodeForError(err error) int {
	if err == nil {
		return ExitSuccess
	}
	switch err.(type) {
	case *ConfigError:
		return ExitConfigError
	case *APIError:
		return ExitAPIError
	case *PartialError:
		return ExitPartialFailure
	case *OutputError:
		return ExitOutputError
	default:
		return ExitAPIError
	}
}
