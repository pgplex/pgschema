package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestLoadDataConfig(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		data, err := LoadDataConfig(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if data != nil {
			t.Fatalf("expected nil data config, got %+v", data)
		}
	})

	t.Run("data tables", func(t *testing.T) {
		dir := t.TempDir()
		content := "[data]\ntables = [\"country\", \"ref_*\", \"!ref_archive\"]\n"
		if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		data, err := LoadDataConfig(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if data == nil {
			t.Fatal("expected data config")
		}
		for name, want := range map[string]bool{
			"country":     true,
			"ref_status":  true,
			"ref_archive": false,
			"users":       false,
		} {
			if got := data.IsDataTable(name); got != want {
				t.Errorf("IsDataTable(%q) = %v, want %v", name, got, want)
			}
		}
	})

	t.Run("empty data section", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte("[data]\ntables = []\n"), 0644); err != nil {
			t.Fatal(err)
		}
		data, err := LoadDataConfig(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if data != nil {
			t.Fatal("expected nil data config for empty table list")
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte("[data]\ntable = [\"x\"]\n"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadDataConfig(dir)
		if err == nil || !strings.Contains(err.Error(), "unknown key") {
			t.Fatalf("expected unknown key error, got %v", err)
		}
	})

	t.Run("invalid toml", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte("[data\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDataConfig(dir); err == nil {
			t.Fatal("expected parse error")
		}
	})
}

func TestValidateDataAgainstIgnore(t *testing.T) {
	data := &ir.DataConfig{Tables: []string{"ref_*"}}
	ignore := &ir.IgnoreConfig{Tables: []string{"ref_legacy"}}

	if err := ValidateDataAgainstIgnore(data, ignore, []string{"ref_status", "users"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := ValidateDataAgainstIgnore(data, ignore, []string{"ref_status", "ref_legacy"})
	if err == nil || !strings.Contains(err.Error(), "ref_legacy") {
		t.Fatalf("expected conflict error for ref_legacy, got %v", err)
	}
	if err := ValidateDataAgainstIgnore(nil, ignore, []string{"ref_legacy"}); err != nil {
		t.Fatalf("nil data config must not error: %v", err)
	}
	if err := ValidateDataAgainstIgnore(data, nil, []string{"ref_legacy"}); err != nil {
		t.Fatalf("nil ignore config must not error: %v", err)
	}
}
