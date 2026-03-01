package cli

// Action represents the CLI action to perform.
type Action int

const (
	ActionRun     Action = iota // Default: run the TUI
	ActionAuth                  // azdo auth: re-set PAT
	ActionHelp                  // azdo --help: show help
	ActionVersion               // azdo --version: show version
)

// String returns the string representation of an Action.
func (a Action) String() string {
	switch a {
	case ActionRun:
		return "run"
	case ActionAuth:
		return "auth"
	case ActionHelp:
		return "help"
	case ActionVersion:
		return "version"
	default:
		return "unknown"
	}
}

// ParseArgs parses command-line arguments and returns the action to perform.
func ParseArgs(args []string) Action {
	if len(args) < 2 {
		return ActionRun
	}

	switch args[1] {
	case "auth":
		return ActionAuth
	case "--help", "-h", "help":
		return ActionHelp
	case "--version", "-v", "version":
		return ActionVersion
	default:
		return ActionRun
	}
}
