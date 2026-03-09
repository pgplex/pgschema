package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIgnoreFile_FileNotExists(t *testing.T) {
	// Ensure no .pgschemaignore file exists in current directory
	os.Remove(".pgschemaignore")

	config, err := LoadIgnoreFile()
	if err != nil {
		t.Fatalf("LoadIgnoreFile() should not error when file doesn't exist, got: %v", err)
	}
	if config != nil {
		t.Error("LoadIgnoreFile() should return nil config when file doesn't exist")
	}
}

func TestLoadIgnoreFileFromPath_ValidTOML(t *testing.T) {
	// Create a temporary TOML file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.pgschemaignore")

	tomlContent := `[tables]
patterns = ["temp_*", "backup_*", "!backup_core"]

[views]
patterns = ["view_temp_*"]

[functions]
patterns = ["fn_test_*", "fn_debug_*"]

[procedures]
patterns = ["sp_temp_*"]

[types]
patterns = ["type_test_*"]

[sequences]
patterns = ["seq_temp_*"]
`

	err := os.WriteFile(testFile, []byte(tomlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadIgnoreFileFromPath(testFile)
	if err != nil {
		t.Fatalf("LoadIgnoreFileFromPath() error = %v", err)
	}
	if config == nil {
		t.Fatal("LoadIgnoreFileFromPath() returned nil config")
	}

	// Test the loaded configuration
	expectedTables := []string{"temp_*", "backup_*", "!backup_core"}
	if len(config.Tables) != len(expectedTables) {
		t.Errorf("Expected %d table patterns, got %d", len(expectedTables), len(config.Tables))
	}
	for i, expected := range expectedTables {
		if config.Tables[i] != expected {
			t.Errorf("Expected table pattern %q at index %d, got %q", expected, i, config.Tables[i])
		}
	}

	// Test other sections
	if len(config.Views) != 1 || config.Views[0] != "view_temp_*" {
		t.Errorf("Expected views patterns [\"view_temp_*\"], got %v", config.Views)
	}

	if len(config.Functions) != 2 {
		t.Errorf("Expected 2 function patterns, got %d", len(config.Functions))
	}

	if len(config.Procedures) != 1 || config.Procedures[0] != "sp_temp_*" {
		t.Errorf("Expected procedure patterns [\"sp_temp_*\"], got %v", config.Procedures)
	}
}

func TestLoadIgnoreFileWithStructure_ValidTOML(t *testing.T) {
	// Create a temporary TOML file using the structured format
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_structured.pgschemaignore")

	tomlContent := `[tables]
patterns = ["temp_*", "backup_*"]

[views]
patterns = ["view_temp_*"]

[functions]
patterns = ["fn_test_*"]
`

	err := os.WriteFile(testFile, []byte(tomlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadIgnoreFileWithStructureFromPath(testFile)
	if err != nil {
		t.Fatalf("LoadIgnoreFileWithStructureFromPath() error = %v", err)
	}
	if config == nil {
		t.Fatal("LoadIgnoreFileWithStructureFromPath() returned nil config")
	}

	// Test the converted configuration
	expectedTables := []string{"temp_*", "backup_*"}
	if len(config.Tables) != len(expectedTables) {
		t.Errorf("Expected %d table patterns, got %d", len(expectedTables), len(config.Tables))
	}
	for i, expected := range expectedTables {
		if config.Tables[i] != expected {
			t.Errorf("Expected table pattern %q at index %d, got %q", expected, i, config.Tables[i])
		}
	}

	if len(config.Views) != 1 || config.Views[0] != "view_temp_*" {
		t.Errorf("Expected views patterns [\"view_temp_*\"], got %v", config.Views)
	}

	if len(config.Functions) != 1 || config.Functions[0] != "fn_test_*" {
		t.Errorf("Expected function patterns [\"fn_test_*\"], got %v", config.Functions)
	}
}

func TestLoadIgnoreFile_PrivilegeSections(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.pgschemaignore")

	tomlContent := `[privileges]
patterns = ["deploy_bot", "admin_*", "!admin_super"]

[default_privileges]
patterns = ["deploy_bot"]
`

	err := os.WriteFile(testFile, []byte(tomlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadIgnoreFileFromPath(testFile)
	if err != nil {
		t.Fatalf("LoadIgnoreFileFromPath() error = %v", err)
	}
	if config == nil {
		t.Fatal("LoadIgnoreFileFromPath() returned nil config")
	}

	// Test privileges section
	expectedPrivileges := []string{"deploy_bot", "admin_*", "!admin_super"}
	if len(config.Privileges) != len(expectedPrivileges) {
		t.Errorf("Expected %d privilege patterns, got %d", len(expectedPrivileges), len(config.Privileges))
	}
	for i, expected := range expectedPrivileges {
		if i < len(config.Privileges) && config.Privileges[i] != expected {
			t.Errorf("Expected privilege pattern %q at index %d, got %q", expected, i, config.Privileges[i])
		}
	}

	// Test default_privileges section
	if len(config.DefaultPrivileges) != 1 || config.DefaultPrivileges[0] != "deploy_bot" {
		t.Errorf("Expected default_privileges patterns [\"deploy_bot\"], got %v", config.DefaultPrivileges)
	}

	// Test ShouldIgnorePrivilege
	if !config.ShouldIgnorePrivilege("deploy_bot") {
		t.Error("deploy_bot should be ignored")
	}
	if !config.ShouldIgnorePrivilege("admin_role") {
		t.Error("admin_role should be ignored (matches admin_*)")
	}
	if config.ShouldIgnorePrivilege("admin_super") {
		t.Error("admin_super should NOT be ignored (negation pattern)")
	}
	if config.ShouldIgnorePrivilege("app_reader") {
		t.Error("app_reader should NOT be ignored")
	}

	// Test ShouldIgnoreDefaultPrivilege
	if !config.ShouldIgnoreDefaultPrivilege("deploy_bot") {
		t.Error("deploy_bot default privilege should be ignored")
	}
	if config.ShouldIgnoreDefaultPrivilege("app_reader") {
		t.Error("app_reader default privilege should NOT be ignored")
	}
}

func TestLoadIgnoreFile_InvalidTOML(t *testing.T) {
	// Create a temporary invalid TOML file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid.pgschemaignore")

	invalidTomlContent := `[tables
patterns = ["temp_*"  # Missing closing bracket and quote
`

	err := os.WriteFile(testFile, []byte(invalidTomlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadIgnoreFileFromPath(testFile)
	if err == nil {
		t.Error("LoadIgnoreFileFromPath() should return error for invalid TOML")
	}
	if config != nil {
		t.Error("LoadIgnoreFileFromPath() should return nil config for invalid TOML")
	}
}

func TestLoadIgnoreFile_EmptyTOML(t *testing.T) {
	// Create a temporary empty TOML file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "empty.pgschemaignore")

	err := os.WriteFile(testFile, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadIgnoreFileFromPath(testFile)
	if err != nil {
		t.Fatalf("LoadIgnoreFileFromPath() should not error for empty TOML, got: %v", err)
	}
	if config == nil {
		t.Fatal("LoadIgnoreFileFromPath() should return empty config for empty TOML")
	}

	// All pattern slices should be empty
	if len(config.Tables) != 0 {
		t.Errorf("Expected empty tables patterns, got %v", config.Tables)
	}
	if len(config.Views) != 0 {
		t.Errorf("Expected empty views patterns, got %v", config.Views)
	}
}

func TestLoadIgnoreFile_PartialTOML(t *testing.T) {
	// Create a temporary TOML file with only some sections
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "partial.pgschemaignore")

	tomlContent := `[tables]
patterns = ["temp_*"]

# Missing other sections
`

	err := os.WriteFile(testFile, []byte(tomlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadIgnoreFileFromPath(testFile)
	if err != nil {
		t.Fatalf("LoadIgnoreFileFromPath() error = %v", err)
	}
	if config == nil {
		t.Fatal("LoadIgnoreFileFromPath() returned nil config")
	}

	// Tables should be populated
	if len(config.Tables) != 1 || config.Tables[0] != "temp_*" {
		t.Errorf("Expected table patterns [\"temp_*\"], got %v", config.Tables)
	}

	// Other sections should be empty
	if len(config.Views) != 0 {
		t.Errorf("Expected empty views patterns, got %v", config.Views)
	}
	if len(config.Functions) != 0 {
		t.Errorf("Expected empty functions patterns, got %v", config.Functions)
	}
}