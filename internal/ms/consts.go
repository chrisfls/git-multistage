package ms

// Strings that several files share.
const (
	flagStage      = "--stage"
	flagAgentic    = "--agentic"
	flagJSON       = "--json"
	flagCached     = "--cached"
	cmdAdd         = "add"
	cmdCommit      = "commit"
	cmdDiff        = "diff"
	cmdLog         = "log"
	cmdUpdateIndex = "update-index"
	jsonError      = "error"
	jsonWarnings   = "warnings"
	agentClaude    = "claude"
	agentCodex     = "codex"
	refHEAD        = "HEAD"
	// trailerMoved names a move of a file in a commit message.
	trailerMoved = "Multistage-Moved"
	// envLiteralPathspecs makes git read pathspecs as literal paths.
	envLiteralPathspecs = "GIT_LITERAL_PATHSPECS=0"
)
