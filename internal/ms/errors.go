package ms

import "fmt"

// Exit codes. Every command uses them, so agents can branch on the status.
const (
	ExitOK       = 0
	ExitError    = 1 // a failure, or nothing to commit
	ExitUsage    = 2 // bad arguments or missing agent ID
	ExitConflict = 3 // staged changes conflict with the branch
	ExitStale    = 4 // a hunk ID does not match the current diff
	ExitRefused  = 5 // the repository is in a state that the command refuses to change
	ExitBusy     = 6 // a lock stayed held until the timeout
)

// Error is a failure with an exit code and details for JSON output.
type Error struct {
	Code    int      `json:"-"`
	Kind    string   `json:"kind"`
	Message string   `json:"message"`
	Paths   []string `json:"paths,omitempty"`
	Details []string `json:"details,omitempty"`
	Hint    string   `json:"hint,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func errorf(code int, kind, format string, args ...any) *Error {
	return &Error{Code: code, Kind: kind, Message: fmt.Sprintf(format, args...)}
}

func usageErr(format string, args ...any) *Error {
	return errorf(ExitUsage, "usage", format, args...)
}
