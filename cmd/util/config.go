package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/ir"
)

const (
	// ConfigFileName is the name of the project configuration file, loaded from
	// the current directory.
	ConfigFileName = "pgschema.toml"

	// legacyIgnoreFileName is the standalone ignore file that the [ignore] table
	// of pgschema.toml replaced.
	legacyIgnoreFileName = ".pgschemaignore"

	// configDocsURL documents the file format; appended to load errors.
	configDocsURL = "https://www.pgschema.com/cli/ignore"
)

// Config represents the structure of pgschema.toml. Each top-level TOML table
// maps to one field.
type Config struct {
	// Ignore lists the objects excluded from dump, plan, and apply. It is nil
	// when the file has no [ignore] table.
	Ignore *ir.IgnoreConfig `toml:"ignore,omitempty"`
}

// LoadConfig loads pgschema.toml from the current directory.
// It returns an empty Config if the file doesn't exist (the file is optional).
func LoadConfig() (*Config, error) {
	return LoadConfigFromPath(ConfigFileName)
}

// LoadConfigFromPath loads a pgschema.toml file from the specified path.
// It returns an empty Config if the file doesn't exist (the file is optional).
func LoadConfigFromPath(filePath string) (*Config, error) {
	// Resolve to an absolute path so errors and the diagnostic logs below
	// unambiguously show where pgschema looked (e.g. when running from the
	// wrong directory).
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	// A leftover .pgschemaignore must not be skipped silently: the objects it
	// used to hide would show up in the next plan as objects to drop.
	legacyPath := filepath.Join(filepath.Dir(absPath), legacyIgnoreFileName)
	if _, err := os.Stat(legacyPath); err == nil {
		return nil, fmt.Errorf("%s is no longer supported: move its rules into the [ignore] table of %s and delete it. "+
			"Each section such as `[tables]` with `patterns = [...]` becomes a `tables = [...]` key under `[ignore]`, "+
			"see %s", legacyPath, ConfigFileName, configDocsURL)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Get().Debug("no config file found, no filtering applied", "file", absPath)
		return &Config{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", absPath, err)
	}

	var config Config
	md, err := toml.DecodeFile(filePath, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w (see %s)", absPath, err, configDocsURL)
	}

	// Reject unknown tables/keys. A typo such as "tabels" would otherwise be
	// dropped silently, leaving nothing ignored.
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, key := range undecoded {
			keys[i] = key.String()
		}
		return nil, fmt.Errorf("unknown key(s) in %s: %s (see %s)", absPath, strings.Join(keys, ", "), configDocsURL)
	}

	logger.Get().Debug("loaded config file", "file", absPath)

	return &config, nil
}
