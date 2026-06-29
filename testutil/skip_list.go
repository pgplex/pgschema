// Package testutil provides shared test utilities for pgschema
package testutil

import (
	"strings"
	"testing"
)

// LatestPostgresVersion is the most recent PostgreSQL major version pgschema
// targets. It is the single point of indirection for the test strategy:
//
//   - The full test suite runs only against this version.
//   - Older versions (14 .. LatestPostgresVersion-1) run only the essential
//     smoke-test subset defined in essentialTests.
//
// When a new PostgreSQL major version ships, bump this constant (and add the new
// version to the CI matrix); the full suite automatically moves to it and the
// previous latest drops down to the essential subset — a single-line change.
const LatestPostgresVersion = 18

// essentialTests is the minimal smoke-test subset run against non-latest
// PostgreSQL versions (14 .. LatestPostgresVersion-1).
//
// Rationale: re-running the entire suite on every supported version mostly
// re-tests version-agnostic logic and forces a large skip list for cosmetic
// differences (e.g. pg_get_viewdef() qualifies columns differently before PG 16)
// and version-gated features (NULLS NOT DISTINCT in PG 15+, temporal constraints
// in PG 18+). Instead we run one or two representative cases per object type on
// older versions — enough to catch gross, version-specific breakage such as a
// catalog query that fails on an older release — and leave exhaustive coverage
// (including views, materialized views, and dependency ordering, whose golden
// files drift across versions) to the full suite on LatestPostgresVersion.
//
// Every entry here MUST have golden files that are byte-stable across all
// supported versions: no view definitions (pg_get_viewdef() drift) and no
// version-gated syntax.
var essentialTests = []string{
	// Tables: create, alter, columns, constraints
	"create_table/add_table",
	"create_table/add_column_integer",
	"create_table/add_column_text",
	"create_table/drop_column",
	"create_table/remove_not_null",
	"create_table/add_check",
	"create_table/add_uk",
	"create_table/add_unique_constraint",

	// Indexes
	"create_index/add_index_with_reloptions",

	// Functions and procedures
	"create_function/add_function",
	"create_function/drop_function",
	"create_procedure/add_procedure",

	// Sequences
	"create_sequence/add_sequence",

	// Types and domains
	"create_type/add_type",
	"create_type/add_value",
	"create_domain/add_domain",

	// Triggers (non-view based)
	"create_trigger/drop_trigger",
	"create_trigger/add_trigger_constraint",

	// Policies
	"create_policy/add_policy",

	// Aggregates
	"create_aggregate/add_aggregate",

	// Comments
	"comment/add_table_comment",
	"comment/add_column_comments",

	// Privileges
	"privilege/grant_table_select",
	"default_privilege/add_table_privilege",
}

// skipListRequiresExtension defines test cases that require third-party extensions
// not available in embedded-postgres (e.g., pgvector, PostGIS).
// These tests are skipped on all PostgreSQL versions in CI but can be run manually
// against a database with the required extensions installed.
var skipListRequiresExtension = []string{
	"create_table/issue_295_pgvector_typmod",
}

// matchesAnyPattern reports whether testName matches any of the given patterns.
// Patterns use a slash before the test name (e.g. "create_view/add_view") while
// test names from subtests use underscores (e.g. "create_view_add_view"), so we
// accept both forms.
func matchesAnyPattern(testName string, patterns []string) bool {
	for _, pattern := range patterns {
		if testName == pattern || testName == strings.ReplaceAll(pattern, "/", "_") {
			return true
		}
	}
	return false
}

// ShouldSkipTest checks if a test should be skipped for the given PostgreSQL major version.
// If the test should be skipped, it calls t.Skipf() which stops test execution.
//
// Strategy:
//   - Tests requiring an unavailable extension are skipped on every version.
//   - The full suite runs only against LatestPostgresVersion.
//   - Older versions run only the essential smoke-test subset (essentialTests).
//
// Test name format examples:
//   - "create_view_add_view" (from TestDiffFromFiles subtests - underscores separate all parts)
//   - "create_view/add_view" (skip list patterns - underscores in category, slash before test)
//   - "TestDumpCommand_Employee" (from dump tests - starts with Test)
func ShouldSkipTest(t *testing.T, testName string, majorVersion int) {
	t.Helper()

	// Extension-required tests are skipped on every version (the extension isn't
	// bundled with embedded-postgres).
	if matchesAnyPattern(testName, skipListRequiresExtension) {
		t.Skipf("Skipping test %q: requires third-party extension not available in embedded-postgres", testName)
	}

	// The full suite runs only against the latest version.
	if majorVersion >= LatestPostgresVersion {
		return
	}

	// Older versions run only the essential smoke-test subset.
	if !matchesAnyPattern(testName, essentialTests) {
		t.Skipf("Skipping test %q on PostgreSQL %d: only the essential subset runs on non-latest versions (full suite runs on PostgreSQL %d)",
			testName, majorVersion, LatestPostgresVersion)
	}
}
