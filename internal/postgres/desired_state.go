// Package postgres provides PostgreSQL functionality for desired state management.
// This file defines the interface for desired state providers (embedded or external databases).
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DesiredStateProvider is an interface that abstracts the desired state database provider.
// It can be implemented by either embedded PostgreSQL or an external database connection.
type DesiredStateProvider interface {
	// GetConnectionDetails returns connection details for IR inspection
	// Returns: host, port, database, username, password
	GetConnectionDetails() (string, int, string, string, string)

	// GetSchemaName returns the actual schema name to inspect.
	// For embedded postgres: returns the temporary schema name (pgschema_tmp_*)
	// For external database: returns the temporary schema name (pgschema_tmp_*)
	GetSchemaName() string

	// ApplySchema applies the desired state SQL to a schema.
	// For embedded postgres: resets the schema (drop/recreate)
	// For external database: creates temporary schema with timestamp suffix
	ApplySchema(ctx context.Context, schema string, sql string) error

	// Stop performs cleanup.
	// For embedded postgres: stops instance and removes temp directory
	// For external database: drops temporary schema (best effort) and closes connection
	Stop() error
}

// GenerateTempSchemaName creates a unique temporary schema name for plan operations.
// The format is: pgschema_tmp_YYYYMMDD_HHMMSS_RRRRRRRR
// where RRRRRRRR is a random 8-character hex string for uniqueness.
// The "_tmp_" marker makes it distinctive and prevents accidental matching with user schemas.
//
// Example: pgschema_tmp_20251030_154501_a3f9d2e1
//
// Panics if random number generation fails (indicates serious system issue).
func GenerateTempSchemaName() string {
	timestamp := time.Now().Format("20060102_150405")

	// Add random suffix for uniqueness (4 bytes = 8 hex characters)
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		// If crypto/rand fails, something is seriously wrong with the system
		panic(fmt.Sprintf("failed to generate random schema name: %v", err))
	}
	randomSuffix := hex.EncodeToString(randomBytes)

	return fmt.Sprintf("pgschema_tmp_%s_%s", timestamp, randomSuffix)
}

// stripSchemaQualifications removes schema qualifications from SQL statements for the specified target schema.
//
// Purpose:
// When applying user-provided SQL to temporary schemas during the plan command, we need to ensure
// that objects are created in the temporary schema (e.g., pgschema_tmp_20251030_154501_123456789)
// rather than in explicitly qualified schemas. PostgreSQL's search_path only affects unqualified
// object names - explicit schema qualifications always override search_path.
//
// Input SQL Sources:
// - pgschema dump command produces schema-agnostic output (no schema qualifications for target schema)
// - Users may manually edit SQL files and add schema qualifications (e.g., public.table)
// - Users may provide SQL from other sources that contains schema qualifications
//
// Behavior:
// This function strips schema qualifications ONLY for the target schema (specified by schemaName),
// while preserving qualifications for other schemas. This allows:
// 1. Target schema objects to be created in temporary schemas via search_path
// 2. Cross-schema references to be preserved correctly
//
// Examples:
// When target schema is "public":
// - public.employees -> employees (stripped - will use search_path)
// - "public".employees -> employees (stripped - handles quoted identifiers)
// - public."employees" -> "employees" (stripped - preserves quoted object names)
// - other_schema.users -> other_schema.users (preserved - cross-schema reference)
//
// It handles both quoted and unquoted schema names:
// - public.table -> table
// - "public".table -> table
// - public."table" -> "table"
// - "public"."table" -> "table"
//
// Only qualifications matching the specified schemaName are stripped.
// All other schema qualifications are preserved as intentional cross-schema references.
func stripSchemaQualifications(sql string, schemaName string) string {
	if schemaName == "" {
		return sql
	}

	// Split SQL into dollar-quoted and non-dollar-quoted segments.
	// Schema qualifiers inside function/procedure bodies (dollar-quoted blocks)
	// must be preserved — the user may need them when search_path doesn't include
	// the function's schema (e.g., SET search_path = ''). (Issue #354)
	segments := splitDollarQuotedSegments(sql)
	var result strings.Builder
	result.Grow(len(sql))
	for _, seg := range segments {
		if seg.quoted {
			// Preserve dollar-quoted content as-is
			result.WriteString(seg.text)
		} else {
			result.WriteString(stripSchemaQualificationsFromText(seg.text, schemaName))
		}
	}
	return result.String()
}

// dollarQuotedSegment represents a segment of SQL text, either inside or outside a dollar-quoted block.
type dollarQuotedSegment struct {
	text   string
	quoted bool // true if this segment is inside dollar quotes (including the delimiters)
}

// splitDollarQuotedSegments splits SQL text into segments that are either inside or outside
// dollar-quoted blocks ($$...$$, $tag$...$tag$, etc.). This allows callers to process
// only the non-quoted parts while preserving function/procedure bodies verbatim.
// dollarQuoteRe matches PostgreSQL dollar-quote tags: $$ or $identifier$ where the
// identifier must start with a letter or underscore (not a digit). This avoids
// false positives on $1, $2 etc. parameter references.
var dollarQuoteRe = regexp.MustCompile(`\$(?:[a-zA-Z_][a-zA-Z0-9_]*)?\$`)

func splitDollarQuotedSegments(sql string) []dollarQuotedSegment {
	var segments []dollarQuotedSegment

	pos := 0
	for pos < len(sql) {
		// Find the next dollar-quote opening tag
		loc := dollarQuoteRe.FindStringIndex(sql[pos:])
		if loc == nil {
			// No more dollar quotes — rest is unquoted
			segments = append(segments, dollarQuotedSegment{text: sql[pos:], quoted: false})
			break
		}

		openStart := pos + loc[0]
		openEnd := pos + loc[1]
		tag := sql[openStart:openEnd]

		// Add the unquoted segment before this tag
		if openStart > pos {
			segments = append(segments, dollarQuotedSegment{text: sql[pos:openStart], quoted: false})
		}

		// Find the matching closing tag
		closeIdx := strings.Index(sql[openEnd:], tag)
		if closeIdx == -1 {
			// No closing tag — treat rest as quoted (unterminated)
			segments = append(segments, dollarQuotedSegment{text: sql[openStart:], quoted: true})
			pos = len(sql)
		} else {
			closeEnd := openEnd + closeIdx + len(tag)
			segments = append(segments, dollarQuotedSegment{text: sql[openStart:closeEnd], quoted: true})
			pos = closeEnd
		}
	}
	return segments
}

// stripSchemaQualificationsFromText performs the actual schema qualification stripping on a text segment.
func stripSchemaQualificationsFromText(text string, schemaName string) string {
	// Escape the schema name for use in regex
	escapedSchema := regexp.QuoteMeta(schemaName)

	// Pattern matches schema qualification and captures the object name
	// We need to handle 4 cases:
	// 1. unquoted_schema.unquoted_object  -> unquoted_object
	// 2. unquoted_schema."quoted_object"  -> "quoted_object"
	// 3. "quoted_schema".unquoted_object  -> unquoted_object
	// 4. "quoted_schema"."quoted_object"  -> "quoted_object"
	//
	// Key: The dot must be outside quotes (a schema.object separator, not part of an identifier)
	// Important: For unquoted schema patterns, we must ensure the schema name isn't inside a quoted identifier
	// Example: Don't match 'public' in CREATE INDEX "public.idx" where the whole thing is a quoted identifier

	// Pattern 1: quoted schema + dot + quoted object: "schema"."object"
	// Example: "public"."table" -> "table"
	pattern1 := fmt.Sprintf(`"%s"\.(\"[^"]+\")`, escapedSchema)
	re1 := regexp.MustCompile(pattern1)

	// Pattern 2: quoted schema + dot + unquoted object: "schema".object
	// Example: "public".table -> table
	pattern2 := fmt.Sprintf(`"%s"\.([a-zA-Z_][a-zA-Z0-9_$]*)`, escapedSchema)
	re2 := regexp.MustCompile(pattern2)

	// Pattern 3: unquoted schema + dot + quoted object: schema."object"
	// Example: public."table" -> "table"
	// Use negative lookbehind to ensure schema isn't preceded by a quote
	// and negative lookahead to ensure the dot after schema isn't inside quotes
	pattern3 := fmt.Sprintf(`(?:^|[^"])%s\.(\"[^"]+\")`, escapedSchema)
	re3 := regexp.MustCompile(pattern3)

	// Pattern 4: unquoted schema + dot + unquoted object: schema.object
	// Example: public.table -> table
	// Use negative lookbehind to ensure schema isn't preceded by a quote
	pattern4 := fmt.Sprintf(`(?:^|[^"])%s\.([a-zA-Z_][a-zA-Z0-9_$]*)`, escapedSchema)
	re4 := regexp.MustCompile(pattern4)

	result := text
	// Apply in order: quoted schema first to avoid double-matching
	result = re1.ReplaceAllString(result, "$1")
	result = re2.ReplaceAllString(result, "$1")
	// For patterns 3 and 4, we need to preserve the character before the schema
	result = re3.ReplaceAllStringFunc(result, func(match string) string {
		// If match starts with a non-quote character, preserve it
		if len(match) > 0 && match[0] != '"' {
			// Extract the quote identifier (everything after schema.)
			parts := strings.SplitN(match, ".", 2)
			if len(parts) == 2 {
				return string(match[0]) + parts[1]
			}
		}
		// Otherwise just return the captured quoted identifier
		parts := strings.SplitN(match, ".", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return match
	})
	result = re4.ReplaceAllStringFunc(result, func(match string) string {
		// If match starts with a non-quote character, preserve it
		if len(match) > 0 && match[0] != '"' {
			// Extract the unquoted identifier (everything after schema.)
			parts := strings.SplitN(match, ".", 2)
			if len(parts) == 2 {
				return string(match[0]) + parts[1]
			}
		}
		// Otherwise just return the captured unquoted identifier
		parts := strings.SplitN(match, ".", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return match
	})

	return result
}

// replaceSchemaInSearchPath replaces the target schema name in SET search_path clauses
// within function/procedure definitions.
//
// Purpose:
// When functions or procedures have SET search_path = public, pg_temp (or similar),
// PostgreSQL uses that search_path during function body validation (for SQL-language functions)
// and execution. When applying to a temporary schema, we need to replace the target schema
// in these clauses so that table references in function bodies can be resolved.
//
// Example (when target schema is "public" and temp schema is "pgschema_tmp_xxx"):
//
//	SET search_path = public, pg_temp  ->  SET search_path = "pgschema_tmp_xxx", pg_temp
//	SET search_path TO public          ->  SET search_path TO "pgschema_tmp_xxx"
//
// This handles both = and TO syntax, quoted and unquoted schema names (case-insensitive),
// and preserves other schemas in the comma-separated list.
//
// Limitations:
//   - Like stripSchemaQualifications and replaceSchemaInDefaultPrivileges, this function
//     operates on the raw SQL string without dollar-quote awareness. A SET search_path
//     inside a $$-quoted function body (e.g., dynamic SQL) would also be rewritten. In
//     practice this is not an issue because such usage is extremely rare, and the round-trip
//     through database inspection and normalizeSchemaNames restores the original schema name.
//   - When targetSchema is "public", replacing it removes "public" from the function's
//     search_path. If the function body references extension objects installed in "public"
//     (e.g., citext), they may not be found. Most extension objects (uuid, jsonb, etc.) live
//     in pg_catalog which is always searched, so this is rarely an issue in practice.
func replaceSchemaInSearchPath(sql string, targetSchema, tempSchema string) string {
	if targetSchema == "" || tempSchema == "" {
		return sql
	}

	replacement := fmt.Sprintf(`"%s"`, tempSchema)

	// Pattern: SET search_path = ... or SET search_path TO ...
	// We match the entire SET search_path clause and replace the target schema within it.
	searchPathPattern := regexp.MustCompile(`(?i)(SET\s+search_path\s*(?:=|TO)\s*)([^\n;]+)`)

	// Pattern to detect trailing function body start in the captured value.
	// When SET search_path and the body are on the same line, the value regex captures both.
	// Handles both AS $$ (dollar-quoted) and BEGIN ATOMIC (SQL-standard, PG14+) syntax.
	bodyStartPattern := regexp.MustCompile(`(?i)\s+(?:AS\s|BEGIN\s+ATOMIC\b)`)

	return searchPathPattern.ReplaceAllStringFunc(sql, func(match string) string {
		loc := searchPathPattern.FindStringSubmatchIndex(match)
		if loc == nil {
			return match
		}
		prefix := match[loc[2]:loc[3]]
		value := match[loc[4]:loc[5]]

		// Separate the search_path value from any trailing function body start
		suffix := ""
		if asLoc := bodyStartPattern.FindStringIndex(value); asLoc != nil {
			suffix = value[asLoc[0]:]
			value = value[:asLoc[0]]
		}

		// Tokenize the comma-separated search_path list and replace matching schemas.
		// This avoids regex pitfalls with quoted identifiers (e.g., "PUBLIC" should not
		// be matched by a case-insensitive unquoted pattern for "public").
		tokens := strings.Split(value, ",")
		for i, token := range tokens {
			trimmed := strings.TrimSpace(token)
			if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
				// Quoted identifier: case-sensitive exact match.
				// "public" matches targetSchema "public", but "PUBLIC" does not
				// (in PostgreSQL, quoted identifiers preserve case).
				inner := trimmed[1 : len(trimmed)-1]
				if inner == targetSchema {
					tokens[i] = strings.Replace(token, trimmed, replacement, 1)
				}
			} else {
				// Unquoted identifier: case-insensitive match.
				// PostgreSQL folds unquoted identifiers to lowercase.
				if strings.EqualFold(trimmed, targetSchema) {
					tokens[i] = strings.Replace(token, trimmed, replacement, 1)
				}
			}
		}

		return prefix + strings.Join(tokens, ",") + suffix
	})
}

// replaceSchemaInDefaultPrivileges replaces schema names in ALTER DEFAULT PRIVILEGES statements.
// This is needed because stripSchemaQualifications only handles "schema.object" patterns,
// not "IN SCHEMA <schema>" clauses used by ALTER DEFAULT PRIVILEGES.
//
// Example:
//
//	ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO app_user;
//
// becomes:
//
//	ALTER DEFAULT PRIVILEGES IN SCHEMA pgschema_tmp_xxx GRANT SELECT ON TABLES TO app_user;
//
// This ensures default privileges are created in the temporary schema where we can inspect them.
func replaceSchemaInDefaultPrivileges(sql string, targetSchema, tempSchema string) string {
	if targetSchema == "" || tempSchema == "" {
		return sql
	}

	escapedTarget := regexp.QuoteMeta(targetSchema)

	// Pattern: IN SCHEMA <schema> (case insensitive for SQL keywords)
	// Handle both quoted and unquoted schema names
	// Pattern 1: IN SCHEMA "schema" (quoted)
	pattern1 := fmt.Sprintf(`(?i)(IN\s+SCHEMA\s+)"%s"`, escapedTarget)
	re1 := regexp.MustCompile(pattern1)
	result := re1.ReplaceAllString(sql, fmt.Sprintf(`${1}"%s"`, tempSchema))

	// Pattern 2: IN SCHEMA schema (unquoted)
	// Use word boundary to avoid partial matches
	pattern2 := fmt.Sprintf(`(?i)(IN\s+SCHEMA\s+)%s\b`, escapedTarget)
	re2 := regexp.MustCompile(pattern2)
	result = re2.ReplaceAllString(result, fmt.Sprintf(`${1}"%s"`, tempSchema))

	return result
}
