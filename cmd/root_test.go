package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/version"
)

func TestRootCommand(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"--help"})

	err := RootCmd.Execute()
	if err != nil {
		t.Errorf("root command with --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Declarative schema migration for Postgres") {
		t.Errorf("expected help output to contain description, got: %s", output)
	}
}

func TestRootCommandWithoutArgs(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{})

	err := RootCmd.Execute()
	if err != nil {
		t.Errorf("root command without args failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Declarative schema migration for Postgres") {
		t.Errorf("expected output to contain description, got: %s", output)
	}
}

func TestRootCommandVersionFlag(t *testing.T) {
	// Reset flags that earlier tests may have set on the shared global RootCmd.
	// pflag does not reset flag values between Parse calls, and cobra checks
	// the help flag before the version flag.
	if err := RootCmd.Flags().Set("help", "false"); err != nil {
		t.Fatalf("failed to reset help flag: %v", err)
	}

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"--version"})

	err := RootCmd.Execute()
	if err != nil {
		t.Errorf("root command with --version failed: %v", err)
	}

	output := buf.String()
	expected := version.App() + "\n"
	if output != expected {
		t.Errorf("expected version output %q, got %q", expected, output)
	}
}

func TestRootCommandHasSubcommands(t *testing.T) {
	commands := RootCmd.Commands()

	expectedCommands := []string{"dump", "plan", "apply"}
	commandNames := make([]string, len(commands))
	for i, cmd := range commands {
		commandNames[i] = cmd.Name()
	}

	for _, expected := range expectedCommands {
		found := false
		for _, actual := range commandNames {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %s not found in: %v", expected, commandNames)
		}
	}
}
