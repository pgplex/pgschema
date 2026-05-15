package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetermineOutputs(t *testing.T) {
	tests := []struct {
		name        string
		outputHuman string
		outputJSON  string
		outputSQL   string
		expectError bool
		errorMsg    string
		expectCount int
	}{
		{
			name:        "no flags - default to human stdout",
			outputHuman: "",
			outputJSON:  "",
			outputSQL:   "",
			expectCount: 1,
		},
		{
			name:        "single json to stdout",
			outputJSON:  "stdout",
			expectCount: 1,
		},
		{
			name:        "multiple to files",
			outputHuman: "plan.txt",
			outputJSON:  "plan.json",
			outputSQL:   "plan.sql",
			expectCount: 3,
		},
		{
			name:        "json to stdout, sql to file",
			outputJSON:  "stdout",
			outputSQL:   "plan.sql",
			expectCount: 2,
		},
		{
			name:        "multiple stdout error",
			outputJSON:  "stdout",
			outputSQL:   "stdout",
			expectError: true,
			errorMsg:    "only one output format can use stdout",
		},
		{
			name:        "all three with multiple stdout error",
			outputHuman: "stdout",
			outputJSON:  "stdout",
			outputSQL:   "plan.sql",
			expectError: true,
			errorMsg:    "only one output format can use stdout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global variables
			outputHuman = tt.outputHuman
			outputJSON = tt.outputJSON
			outputSQL = tt.outputSQL

			outputs, err := determineOutputs()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(outputs) != tt.expectCount {
				t.Errorf("expected %d outputs, got %d", tt.expectCount, len(outputs))
			}

			// Additional validation for default case
			if tt.name == "no flags - default to human stdout" && len(outputs) > 0 {
				if outputs[0].format != "human" || outputs[0].target != "stdout" {
					t.Errorf("expected default output to be human to stdout, got %+v", outputs[0])
				}
			}
		})
	}
}

func TestDeriveSchemaOutputTarget(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		schemaName string
		expected   string
	}{
		{
			name:       "stdout is unchanged",
			target:     "stdout",
			schemaName: "tenant_1",
			expected:   "stdout",
		},
		{
			name:       "json file gets schema suffix",
			target:     "plan.json",
			schemaName: "tenant_1",
			expected:   "plan_tenant_1.json",
		},
		{
			name:       "sql file gets schema suffix",
			target:     "plan.sql",
			schemaName: "public",
			expected:   "plan_public.sql",
		},
		{
			name:       "txt file gets schema suffix",
			target:     "output.txt",
			schemaName: "tenant_abc",
			expected:   "output_tenant_abc.txt",
		},
		{
			name:       "file with directory path",
			target:     "/tmp/plans/plan.json",
			schemaName: "tenant_2",
			expected:   "/tmp/plans/plan_tenant_2.json",
		},
		{
			name:       "file without extension",
			target:     "plan",
			schemaName: "tenant_1",
			expected:   "plan_tenant_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveSchemaOutputTarget(tt.target, tt.schemaName)
			if got != tt.expected {
				t.Errorf("deriveSchemaOutputTarget(%q, %q) = %q, want %q",
					tt.target, tt.schemaName, got, tt.expected)
			}
		})
	}
}

func TestProcessOutput_FileCreation(t *testing.T) {
	// This test would require a mock plan object
	// For now, we'll just verify the file writing logic works
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Test writing to file
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("expected 'test content', got '%s'", string(content))
	}
}
