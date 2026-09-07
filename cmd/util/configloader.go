package util

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/ir"
)

// ConfigFileName is the name of the project configuration file. It is
// loaded from the current directory, like .pgschemaignore.
const ConfigFileName = "pgschema.toml"

// projectConfig is the TOML structure of pgschema.toml.
type projectConfig struct {
	Data ir.DataConfig `toml:"data,omitempty"`
}

// LoadDataConfig loads the [data] section of pgschema.toml from dir, or from
// the current directory when dir is empty. It returns nil when the file does
// not exist or lists no tables.
func LoadDataConfig(dir string) (*ir.DataConfig, error) {
	path := ConfigFileName
	if dir != "" {
		path = filepath.Join(dir, ConfigFileName)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Get().Debug("no config file found", "file", absPath)
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var config projectConfig
	meta, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", absPath, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown key %q in %s", undecoded[0].String(), absPath)
	}
	logger.Get().Debug("loaded config file", "file", absPath)

	if len(config.Data.Tables) == 0 {
		return nil, nil
	}
	return &config.Data, nil
}

// ValidateDataAgainstIgnore rejects tables that are both data-managed and
// ignored, since the two are contradictory.
func ValidateDataAgainstIgnore(dataConfig *ir.DataConfig, ignoreConfig *ir.IgnoreConfig, tableNames []string) error {
	if dataConfig == nil || ignoreConfig == nil {
		return nil
	}
	for _, name := range tableNames {
		if dataConfig.IsDataTable(name) && ignoreConfig.ShouldIgnoreTable(name) {
			return fmt.Errorf("table %q is listed under [data] in %s and matched by [tables] in %s; a table cannot be both managed and ignored", name, ConfigFileName, IgnoreFileName)
		}
	}
	return nil
}
