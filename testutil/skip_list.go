// Package testutil provides shared test utilities for pgschema
package testutil

import (
	"strings"
	"testing"
)

// skipListPG14_15 defines test cases that should be skipped for PostgreSQL 14-15.
//
// Reason for skipping:
// PostgreSQL 14-15 use pg_get_viewdef() which returns table-qualified column names (e.g., employees.id),
// while PostgreSQL 16+ returns simplified column names (e.g., id). This is a non-consequential
// formatting difference that does not impact correctness, but causes test comparison failures.
var skipListPG14_15 = []string{
	// View tests - pg_get_viewdef() formatting differences
	"create_view/add_view",
	"create_view/alter_view",
	"create_view/drop_view",
	"dependency/table_to_view",

	// Materialized view tests - same pg_get_viewdef() issue
	"create_materialized_view/add_materialized_view",
	"create_materialized_view/alter_materialized_view",
	"create_materialized_view/drop_materialized_view",
	"dependency/table_to_materialized_view",
	"dependency/issue_300_function_table_composite_type",

	// Online materialized view index tests - depend on materialized view definitions
	"online/add_materialized_view_index",
	"online/alter_materialized_view_index",

	// Comment tests - fingerprint includes view definitions
	"comment/add_index_comment",
	"comment/add_view_comment",

	// Index tests - fingerprint includes view definitions
	"create_index/drop_index",

	// Trigger tests - fingerprint includes view definitions
	"create_trigger/add_trigger",

	// Migration tests - include views and materialized views
	"migrate/v4",
	"migrate/v5",

	// Dump integration tests - contain views with formatting differences
	"TestDumpCommand_Employee",
	"TestDumpCommand_Issue82ViewLogicExpr",
	"TestDumpCommand_Issue307ViewDependencyOrder",

	// Include integration test - uses materialized views
	"TestIncludeIntegration",
}

// skipListRequiresExtension defines test cases that require third-party extensions
// not available in embedded-postgres (e.g., pgvector, PostGIS).
// These tests are skipped on all PostgreSQL versions in CI but can be run manually
// against a database with the required extensions installed.
var skipListRequiresExtension = []string{
	"create_table/issue_295_pgvector_typmod",
}

// skipListPG14 defines test cases that should be skipped for PostgreSQL 14 only.
//
// Reason for skipping:
// These tests use features not available in PostgreSQL 14 (e.g., NULLS NOT DISTINCT is PG15+).
var skipListPG14 = []string{
	"create_index/add_index",
}

// skipListForVersion maps PostgreSQL major versions to their skip lists.
var skipListForVersion = map[int][]string{
	14: append(skipListPG14_15, skipListPG14...),
	15: skipListPG14_15,
}

// ShouldSkipTest checks if a test should be skipped for the given PostgreSQL major version.
// If the test should be skipped, it calls t.Skipf() which stops test execution.
//
// Test name format examples:
//   - "create_view_add_view" (from TestDiffFromFiles subtests - underscores separate all parts)
//   - "create_view/add_view" (skip list patterns - underscores in category, slash before test)
//   - "TestDumpCommand_Employee" (from dump tests - starts with Test)
//
// Matching uses exact string match with flexible slash/underscore handling:
// Pattern "create_view/add_view" matches test name "create_view_add_view" (underscores)
func ShouldSkipTest(t *testing.T, testName string, majorVersion int) {
	t.Helper()

	// Check extension-required skip list (applies to all versions)
	for _, pattern := range skipListRequiresExtension {
		patternNormalized := strings.ReplaceAll(pattern, "/", "_")
		if testName == patternNormalized || testName == pattern {
			t.Skipf("Skipping test %q: requires third-party extension not available in embedded-postgres", testName)
		}
	}

	// Get skip patterns for this version
	skipPatterns, exists := skipListForVersion[majorVersion]
	if !exists {
		return // No skips defined for this version
	}

	// Check if test name matches any skip pattern (exact match)
	for _, pattern := range skipPatterns {
		// Convert pattern slashes to underscores to match test name format
		// e.g., "create_view/add_view" -> "create_view_add_view"
		patternNormalized := strings.ReplaceAll(pattern, "/", "_")

		if testName == patternNormalized || testName == pattern {
			t.Skipf("Skipping test %q on PostgreSQL %d (unsupported feature or formatting differences)", testName, majorVersion)
		}
	}
}
