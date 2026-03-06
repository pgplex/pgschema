package ir

import (
	"path/filepath"
	"strings"
)

// IgnoreConfig represents the configuration for ignoring database objects
type IgnoreConfig struct {
	Tables            []string `toml:"tables,omitempty"`
	Views             []string `toml:"views,omitempty"`
	Functions         []string `toml:"functions,omitempty"`
	Procedures        []string `toml:"procedures,omitempty"`
	Types             []string `toml:"types,omitempty"`
	Sequences         []string `toml:"sequences,omitempty"`
	Privileges        []string `toml:"privileges,omitempty"`
	DefaultPrivileges []string `toml:"default_privileges,omitempty"`
}

// ShouldIgnoreTable checks if a table should be ignored based on the patterns
func (c *IgnoreConfig) ShouldIgnoreTable(tableName string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(tableName, c.Tables)
}

// ShouldIgnoreView checks if a view should be ignored based on the patterns
func (c *IgnoreConfig) ShouldIgnoreView(viewName string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(viewName, c.Views)
}

// ShouldIgnoreFunction checks if a function should be ignored based on the patterns
func (c *IgnoreConfig) ShouldIgnoreFunction(functionName string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(functionName, c.Functions)
}

// ShouldIgnoreProcedure checks if a procedure should be ignored based on the patterns
func (c *IgnoreConfig) ShouldIgnoreProcedure(procedureName string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(procedureName, c.Procedures)
}

// ShouldIgnoreType checks if a type should be ignored based on the patterns
func (c *IgnoreConfig) ShouldIgnoreType(typeName string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(typeName, c.Types)
}

// ShouldIgnoreSequence checks if a sequence should be ignored based on the patterns
func (c *IgnoreConfig) ShouldIgnoreSequence(sequenceName string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(sequenceName, c.Sequences)
}

// ShouldIgnorePrivilege checks if a privilege should be ignored based on the grantee role name
func (c *IgnoreConfig) ShouldIgnorePrivilege(grantee string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(grantee, c.Privileges)
}

// ShouldIgnoreDefaultPrivilege checks if a default privilege should be ignored based on the grantee role name
func (c *IgnoreConfig) ShouldIgnoreDefaultPrivilege(grantee string) bool {
	if c == nil {
		return false
	}
	return c.shouldIgnore(grantee, c.DefaultPrivileges)
}

// shouldIgnore checks if a name should be ignored based on the patterns
// Patterns support wildcards (*) and negation (!)
// Negation patterns (starting with !) take precedence over inclusion patterns
func (c *IgnoreConfig) shouldIgnore(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	matched := false

	// First pass: check for positive matches (inclusion patterns)
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "!") {
			continue // Skip negation patterns in first pass
		}

		if matchPattern(pattern, name) {
			matched = true
			break
		}
	}

	// Second pass: check for negation patterns (exclusion from ignore)
	for _, pattern := range patterns {
		if !strings.HasPrefix(pattern, "!") {
			continue // Skip non-negation patterns in second pass
		}

		negPattern := pattern[1:] // Remove the '!' prefix
		if matchPattern(negPattern, name) {
			// Negation pattern matches, so don't ignore this item
			return false
		}
	}

	return matched
}

// matchPattern matches a glob-style pattern against a string
// Supports * wildcard matching
func matchPattern(pattern, name string) bool {
	// Use filepath.Match for glob pattern matching
	matched, err := filepath.Match(pattern, name)
	if err != nil {
		// If pattern is invalid, treat it as a literal match
		return pattern == name
	}
	return matched
}