package ir

import (
	"strings"
	"unicode"
)

// PostgreSQL reserved words that need quoting
// Based on PostgreSQL 17 documentation: https://www.postgresql.org/docs/current/sql-keywords-appendix.html
var reservedWords = map[string]bool{
	// A-C
	"all":              true,
	"analyse":          true,
	"analyze":          true,
	"and":              true,
	"any":              true,
	"array":            true,
	"as":               true,
	"asc":              true,
	"asymmetric":       true,
	"authorization":    true,
	"between":          true,
	"bigint":           true,
	"binary":           true,
	"by":               true,
	"boolean":          true,
	"both":             true,
	"case":             true,
	"cast":             true,
	"char":             true,
	"character":        true,
	"check":            true,
	"collate":          true,
	"collation":        true,
	"column":           true,
	"concurrently":     true,
	"constraint":       true,
	"create":           true,
	"cross":            true,
	"current_catalog":  true,
	"current_date":     true,
	"current_role":     true,
	"current_schema":   true,
	"current_time":     true,
	"current_timestamp": true,
	"current_user":     true,
	// D-F
	"default":     true,
	"deferrable":  true,
	"delete":      true,
	"desc":        true,
	"distinct":    true,
	"do":          true,
	"else":        true,
	"end":         true,
	"except":      true,
	"exists":      true,
	"false":       true,
	"fetch":       true,
	"filter":      true,
	"for":         true,
	"foreign":     true,
	"freeze":      true,
	"from":        true,
	"full":        true,
	// G-L
	"grant":          true,
	"group":          true,
	"having":         true,
	"ilike":          true,
	"in":             true,
	"initially":      true,
	"inner":          true,
	"insert":         true,
	"intersect":      true,
	"into":           true,
	"is":             true,
	"isnull":         true,
	"join":           true,
	"lateral":        true,
	"leading":        true,
	"left":           true,
	"like":           true,
	"limit":          true,
	"localtime":      true,
	"localtimestamp":  true,
	// N-P
	"natural":     true,
	"not":         true,
	"notnull":     true,
	"null":        true,
	"of":          true,
	"offset":      true,
	"on":          true,
	"only":        true,
	"or":          true,
	"order":       true,
	"outer":       true,
	"overlaps":    true,
	"placing":     true,
	"primary":     true,
	// R-S
	"references":   true,
	"returning":    true,
	"right":        true,
	"select":       true,
	"session_user": true,
	"similar":      true,
	"some":         true,
	"symmetric":    true,
	"system_user":  true,
	// T-W
	"table":       true,
	"tablesample": true,
	"then":        true,
	"to":          true,
	"trailing":    true,
	"true":        true,
	"union":       true,
	"update":      true,
	"unique":      true,
	"user":        true,
	"using":       true,
	"variadic":    true,
	"verbose":     true,
	"when":        true,
	"where":       true,
	"window":      true,
	"with":        true,
	"within":      true,
}

// needsQuoting checks if an identifier needs to be quoted
func needsQuoting(identifier string) bool {
	if identifier == "" {
		return false
	}

	// Check if it's a reserved word
	if reservedWords[strings.ToLower(identifier)] {
		return true
	}

	// Check if it contains uppercase letters (PostgreSQL folds unquoted to lowercase)
	for _, r := range identifier {
		if unicode.IsUpper(r) {
			return true
		}
	}

	// Check if it starts with non-letter or contains special characters
	for i, r := range identifier {
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			return true
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return true
		}
	}

	return false
}

// QuoteIdentifier adds quotes to an identifier if needed
func QuoteIdentifier(identifier string) string {
	if needsQuoting(identifier) {
		return `"` + identifier + `"`
	}
	return identifier
}

// QualifyEntityNameWithQuotes returns the properly qualified and quoted entity name.
// The schema qualifier is omitted when the entity lives in the target schema ("smart
// qualification").
func QualifyEntityNameWithQuotes(entitySchema, entityName, targetSchema string) string {
	return QualifyEntityNameWithQuotesMode(entitySchema, entityName, targetSchema, false)
}

// QualifyEntityNameWithQuotesMode is like QualifyEntityNameWithQuotes, but when
// qualifySchema is true the schema qualifier is always emitted — even for entities in
// the target schema (used by `dump --qualify-schema`).
func QualifyEntityNameWithQuotesMode(entitySchema, entityName, targetSchema string, qualifySchema bool) string {
	quotedName := QuoteIdentifier(entityName)

	if !qualifySchema && entitySchema == targetSchema {
		return quotedName
	}

	quotedSchema := QuoteIdentifier(entitySchema)
	return quotedSchema + "." + quotedName
}