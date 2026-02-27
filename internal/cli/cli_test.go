package cli

import "testing"

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Action
	}{
		{"no args runs TUI", []string{"azdo"}, ActionRun},
		{"auth subcommand", []string{"azdo", "auth"}, ActionAuth},
		{"--help flag", []string{"azdo", "--help"}, ActionHelp},
		{"-h flag", []string{"azdo", "-h"}, ActionHelp},
		{"help subcommand", []string{"azdo", "help"}, ActionHelp},
		{"--version flag", []string{"azdo", "--version"}, ActionVersion},
		{"-v flag", []string{"azdo", "-v"}, ActionVersion},
		{"version subcommand", []string{"azdo", "version"}, ActionVersion},
		{"unknown arg defaults to run", []string{"azdo", "foo"}, ActionRun},
		{"extra args after auth ignored", []string{"azdo", "auth", "extra"}, ActionAuth},
		{"empty args defaults to run", []string{}, ActionRun},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseArgs(tt.args)
			if got != tt.want {
				t.Errorf("ParseArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestActionString(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionRun, "run"},
		{ActionAuth, "auth"},
		{ActionHelp, "help"},
		{ActionVersion, "version"},
		{Action(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.action.String()
			if got != tt.want {
				t.Errorf("Action(%d).String() = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}
