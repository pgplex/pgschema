package util

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// writeConfig writes content to a pgschema.toml in a fresh temp directory and
// returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	return path
}

func TestLoadConfig_FileNotExists(t *testing.T) {
	t.Chdir(t.TempDir())

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() should not error when file doesn't exist, got: %v", err)
	}
	if config == nil {
		t.Fatal("LoadConfig() should return an empty config when file doesn't exist")
	}
	if config.Ignore != nil {
		t.Errorf("LoadConfig() should leave Ignore nil when file doesn't exist, got %+v", config.Ignore)
	}
}

func TestLoadConfig_CurrentDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(ConfigFileName, []byte("[ignore]\ntables = [\"temp_*\"]\n"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !config.Ignore.ShouldIgnoreTable("temp_cache") {
		t.Error("temp_cache should be ignored")
	}
}

func TestLoadConfig_Ignore(t *testing.T) {
	path := writeConfig(t, `[ignore]
schemas = ["auth"]
tables = ["temp_*", "backup_*", "!backup_core"]
views = ["view_temp_*"]
functions = ["fn_test_*", "fn_debug_*"]
procedures = ["sp_temp_*"]
aggregates = ["agg_test_*"]
types = ["type_test_*"]
sequences = ["seq_temp_*"]
indexes = ["idx_temp_*"]
constraints = ["fk_temp_*"]
triggers = ["trg_temp_*"]
privileges = ["deploy_bot", "admin_*", "!admin_super"]
default_privileges = ["deploy_bot"]
`)

	config, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadConfigFromPath() error = %v", err)
	}

	want := &ir.IgnoreConfig{
		Schemas:           []string{"auth"},
		Tables:            []string{"temp_*", "backup_*", "!backup_core"},
		Views:             []string{"view_temp_*"},
		Functions:         []string{"fn_test_*", "fn_debug_*"},
		Procedures:        []string{"sp_temp_*"},
		Aggregates:        []string{"agg_test_*"},
		Types:             []string{"type_test_*"},
		Sequences:         []string{"seq_temp_*"},
		Indexes:           []string{"idx_temp_*"},
		Constraints:       []string{"fk_temp_*"},
		Triggers:          []string{"trg_temp_*"},
		Privileges:        []string{"deploy_bot", "admin_*", "!admin_super"},
		DefaultPrivileges: []string{"deploy_bot"},
	}
	if !reflect.DeepEqual(config.Ignore, want) {
		t.Errorf("Ignore = %+v, want %+v", config.Ignore, want)
	}

	// Patterns and negation are applied by the loaded config.
	if !config.Ignore.ShouldIgnorePrivilege("admin_role") {
		t.Error("admin_role should be ignored (matches admin_*)")
	}
	if config.Ignore.ShouldIgnorePrivilege("admin_super") {
		t.Error("admin_super should NOT be ignored (negation pattern)")
	}
	if !config.Ignore.ShouldIgnoreDefaultPrivilege("deploy_bot") {
		t.Error("deploy_bot default privilege should be ignored")
	}
	if config.Ignore.ShouldIgnoreDefaultPrivilege("app_reader") {
		t.Error("app_reader default privilege should NOT be ignored")
	}
}

func TestLoadConfig_PartialIgnore(t *testing.T) {
	path := writeConfig(t, `[ignore]
tables = ["temp_*"]

# Other object types are not listed
`)

	config, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadConfigFromPath() error = %v", err)
	}

	want := &ir.IgnoreConfig{Tables: []string{"temp_*"}}
	if !reflect.DeepEqual(config.Ignore, want) {
		t.Errorf("Ignore = %+v, want %+v", config.Ignore, want)
	}
}

func TestLoadConfig_Empty(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "empty file", content: ""},
		{name: "empty ignore table", content: "[ignore]\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := LoadConfigFromPath(writeConfig(t, tt.content))
			if err != nil {
				t.Fatalf("LoadConfigFromPath() should not error, got: %v", err)
			}
			if config == nil {
				t.Fatal("LoadConfigFromPath() returned nil config")
			}
			// Nothing is ignored, whether Ignore is nil or empty.
			if config.Ignore.ShouldIgnoreTable("users") {
				t.Error("users should NOT be ignored")
			}
		})
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	path := writeConfig(t, `[ignore
tables = ["temp_*"  # Missing closing brackets
`)

	config, err := LoadConfigFromPath(path)
	if err == nil {
		t.Fatal("LoadConfigFromPath() should return error for invalid TOML")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q should mention the file path %q", err.Error(), path)
	}
	if config != nil {
		t.Error("LoadConfigFromPath() should return nil config for invalid TOML")
	}
}

func TestLoadConfig_UnknownKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantKey string
	}{
		{
			name:    "misspelled object type",
			content: "[ignore]\nveiws = [\"batch_stack_v\"]\n",
			wantKey: "ignore.veiws",
		},
		{
			name:    "unknown table",
			content: "[ingore]\nviews = [\"batch_stack_v\"]\n",
			wantKey: "ingore",
		},
		{
			// A .pgschemaignore renamed to pgschema.toml without rewriting it.
			name:    "legacy top-level section",
			content: "[views]\npatterns = [\"batch_stack_v\"]\n",
			wantKey: "views",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := LoadConfigFromPath(writeConfig(t, tt.content))
			if err == nil {
				t.Fatal("LoadConfigFromPath() should return error for unknown keys")
			}
			if !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("error %q should mention %q", err.Error(), tt.wantKey)
			}
			if config != nil {
				t.Error("LoadConfigFromPath() should return nil config for unknown keys")
			}
		})
	}
}

// A .pgschemaignore renamed to pgschema.toml with only the section headers
// prefixed must fail rather than load with nothing ignored.
func TestLoadConfig_LegacyNestedSection(t *testing.T) {
	path := writeConfig(t, "[ignore.views]\npatterns = [\"batch_stack_v\"]\n")

	config, err := LoadConfigFromPath(path)
	if err == nil {
		t.Fatal("LoadConfigFromPath() should return error for [ignore.views] sections")
	}
	if config != nil {
		t.Error("LoadConfigFromPath() should return nil config for [ignore.views] sections")
	}
}

// A leftover .pgschemaignore must stop the command: skipping it silently would
// turn every previously ignored object into a planned change.
func TestLoadConfig_LegacyIgnoreFile(t *testing.T) {
	tests := []struct {
		name       string
		withConfig bool
	}{
		{name: "legacy file only"},
		{name: "legacy file next to pgschema.toml", withConfig: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			legacyPath := filepath.Join(dir, ".pgschemaignore")
			if err := os.WriteFile(legacyPath, []byte("[tables]\npatterns = [\"temp_*\"]\n"), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}
			configPath := filepath.Join(dir, ConfigFileName)
			if tt.withConfig {
				if err := os.WriteFile(configPath, []byte("[ignore]\ntables = [\"temp_*\"]\n"), 0644); err != nil {
					t.Fatalf("Failed to write test file: %v", err)
				}
			}

			config, err := LoadConfigFromPath(configPath)
			if err == nil {
				t.Fatal("LoadConfigFromPath() should return error when .pgschemaignore exists")
			}
			for _, want := range []string{legacyPath, "no longer supported as of pgschema 2.0.0", "[ignore]", ConfigFileName} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should mention %q", err.Error(), want)
				}
			}
			if config != nil {
				t.Error("LoadConfigFromPath() should return nil config when .pgschemaignore exists")
			}
		})
	}
}

// The guard looks at the .pgschemaignore entry itself, so a symlink whose
// target is missing still counts as a leftover file.
func TestLoadConfig_LegacyIgnoreFileDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, ".pgschemaignore")
	if err := os.Symlink(filepath.Join(dir, "missing"), legacyPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	config, err := LoadConfigFromPath(filepath.Join(dir, ConfigFileName))
	if err == nil {
		t.Fatal("LoadConfigFromPath() should return error for a dangling .pgschemaignore symlink")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("error %q should be the migration error", err.Error())
	}
	if config != nil {
		t.Error("LoadConfigFromPath() should return nil config for a dangling .pgschemaignore symlink")
	}
}

// A failure to look up .pgschemaignore other than "not found" must not be read
// as "no legacy file": the command would then run without the ignore rules.
func TestLoadConfig_LegacyIgnoreFileLookupError(t *testing.T) {
	// A regular file where the directory should be makes the lookup fail with
	// ENOTDIR on every platform, without depending on permissions.
	notDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDir, nil, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := LoadConfigFromPath(filepath.Join(notDir, ConfigFileName))
	if err == nil {
		t.Fatal("LoadConfigFromPath() should return error when the .pgschemaignore lookup fails")
	}
	if !strings.Contains(err.Error(), ".pgschemaignore") {
		t.Errorf("error %q should mention .pgschemaignore", err.Error())
	}
	if config != nil {
		t.Error("LoadConfigFromPath() should return nil config when the .pgschemaignore lookup fails")
	}
}

// A pgschema.toml symlink whose target is missing is a broken config, not an
// absent one: loading it as empty would drop every ignore rule.
func TestLoadConfig_DanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.Symlink(filepath.Join(dir, "missing"), path); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	config, err := LoadConfigFromPath(path)
	if err == nil {
		t.Fatal("LoadConfigFromPath() should return error for a dangling pgschema.toml symlink")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q should mention the file path %q", err.Error(), path)
	}
	if config != nil {
		t.Error("LoadConfigFromPath() should return nil config for a dangling pgschema.toml symlink")
	}
}
