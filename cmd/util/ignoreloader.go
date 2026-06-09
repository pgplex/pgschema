package util

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/ir"
)

const (
	// IgnoreFileName is the default name of the ignore file
	IgnoreFileName = ".pgschemaignore"
)

// LoadIgnoreFile loads the .pgschemaignore file from the current directory
// Returns nil if the file doesn't exist (ignore functionality is optional)
func LoadIgnoreFile() (*ir.IgnoreConfig, error) {
	return LoadIgnoreFileFromPath(IgnoreFileName)
}

// LoadIgnoreFileFromPath loads an ignore file from the specified path
// Returns nil if the file doesn't exist (ignore functionality is optional)
// Uses the structured TOML format internally
func LoadIgnoreFileFromPath(filePath string) (*ir.IgnoreConfig, error) {
	return LoadIgnoreFileWithStructureFromPath(filePath)
}

// TomlConfig represents the TOML structure of the .pgschemaignore file
// This is used for parsing more complex configurations if needed in the future
type TomlConfig struct {
	Tables            TableIgnoreConfig            `toml:"tables,omitempty"`
	Views             ViewIgnoreConfig             `toml:"views,omitempty"`
	Functions         FunctionIgnoreConfig         `toml:"functions,omitempty"`
	Procedures        ProcedureIgnoreConfig        `toml:"procedures,omitempty"`
	Aggregates        AggregateIgnoreConfig        `toml:"aggregates,omitempty"`
	Types             TypeIgnoreConfig             `toml:"types,omitempty"`
	Sequences         SequenceIgnoreConfig         `toml:"sequences,omitempty"`
	Indexes           IndexIgnoreConfig            `toml:"indexes,omitempty"`
	Constraints       ConstraintIgnoreConfig       `toml:"constraints,omitempty"`
	Triggers          TriggerIgnoreConfig          `toml:"triggers,omitempty"`
	Privileges        PrivilegeIgnoreConfig        `toml:"privileges,omitempty"`
	DefaultPrivileges DefaultPrivilegeIgnoreConfig `toml:"default_privileges,omitempty"`
}

// TableIgnoreConfig represents table-specific ignore configuration
type TableIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// ViewIgnoreConfig represents view-specific ignore configuration
type ViewIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// FunctionIgnoreConfig represents function-specific ignore configuration
type FunctionIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// ProcedureIgnoreConfig represents procedure-specific ignore configuration
type ProcedureIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// AggregateIgnoreConfig represents aggregate-specific ignore configuration
type AggregateIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// TypeIgnoreConfig represents type-specific ignore configuration
type TypeIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// SequenceIgnoreConfig represents sequence-specific ignore configuration
type SequenceIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// IndexIgnoreConfig represents index-specific ignore configuration
type IndexIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// ConstraintIgnoreConfig represents constraint-specific ignore configuration
// Patterns match on constraint names
type ConstraintIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// TriggerIgnoreConfig represents trigger-specific ignore configuration
// Patterns match on trigger names
type TriggerIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// PrivilegeIgnoreConfig represents privilege-specific ignore configuration
// Patterns match on grantee role names
type PrivilegeIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// DefaultPrivilegeIgnoreConfig represents default privilege-specific ignore configuration
// Patterns match on grantee role names
type DefaultPrivilegeIgnoreConfig struct {
	Patterns []string `toml:"patterns,omitempty"`
}

// LoadIgnoreFileWithStructure loads the .pgschemaignore file using the structured TOML format
// and converts it to the simple IgnoreConfig structure
func LoadIgnoreFileWithStructure() (*ir.IgnoreConfig, error) {
	return LoadIgnoreFileWithStructureFromPath(IgnoreFileName)
}

// LoadIgnoreFileWithStructureFromPath loads an ignore file using structured format from the specified path
func LoadIgnoreFileWithStructureFromPath(filePath string) (*ir.IgnoreConfig, error) {
	// Resolve to an absolute path so the diagnostic logs below unambiguously
	// show where pgschema looked (e.g. when running from the wrong directory).
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File doesn't exist, return nil config (no filtering).
		logger.Get().Info("no ignore file found, no filtering applied",
			"file", absPath)
		return nil, nil
	} else if err != nil {
		// Other error accessing file
		return nil, err
	}

	// File exists, parse it
	var tomlConfig TomlConfig
	if _, err := toml.DecodeFile(filePath, &tomlConfig); err != nil {
		return nil, err
	}

	logger.Get().Debug("loaded ignore file", "file", absPath)

	// Convert to simple IgnoreConfig structure
	config := &ir.IgnoreConfig{
		Tables:            tomlConfig.Tables.Patterns,
		Views:             tomlConfig.Views.Patterns,
		Functions:         tomlConfig.Functions.Patterns,
		Procedures:        tomlConfig.Procedures.Patterns,
		Aggregates:        tomlConfig.Aggregates.Patterns,
		Types:             tomlConfig.Types.Patterns,
		Sequences:         tomlConfig.Sequences.Patterns,
		Indexes:           tomlConfig.Indexes.Patterns,
		Constraints:       tomlConfig.Constraints.Patterns,
		Triggers:          tomlConfig.Triggers.Patterns,
		Privileges:        tomlConfig.Privileges.Patterns,
		DefaultPrivileges: tomlConfig.DefaultPrivileges.Patterns,
	}

	return config, nil
}
