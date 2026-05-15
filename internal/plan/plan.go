package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pgplex/pgschema/internal/version"
)

// Plan is the top-level migration plan file. It always uses the unified
// schemas map format, even for single-schema operations (one entry).
type Plan struct {
	Version         string                   `json:"version"`
	PgschemaVersion string                   `json:"pgschema_version"`
	CreatedAt       time.Time                `json:"created_at"`
	Schemas         map[string]*SchemaPlan   `json:"schemas"`
}

// NewPlan creates an empty Plan with version metadata and current timestamp.
func NewPlan() *Plan {
	createdAt := time.Now().Truncate(time.Second)
	if testTime := os.Getenv("PGSCHEMA_TEST_TIME"); testTime != "" {
		if parsedTime, err := time.Parse(time.RFC3339, testTime); err == nil {
			createdAt = parsedTime
		}
	}
	return &Plan{
		Version:         version.PlanFormat(),
		PgschemaVersion: version.App(),
		CreatedAt:       createdAt,
		Schemas:         make(map[string]*SchemaPlan),
	}
}

// AddSchema adds a per-schema plan to the Plan.
func (p *Plan) AddSchema(schemaName string, sp *SchemaPlan) {
	p.Schemas[schemaName] = sp
}

// HasAnyChanges returns true if any schema plan has changes.
func (p *Plan) HasAnyChanges() bool {
	for _, sp := range p.Schemas {
		if sp.HasAnyChanges() {
			return true
		}
	}
	return false
}

// SortedSchemaNames returns schema names in sorted order for deterministic iteration.
func (p *Plan) SortedSchemaNames() []string {
	names := make([]string, 0, len(p.Schemas))
	for name := range p.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ToJSON returns the Plan as structured JSON.
func (p *Plan) ToJSON() (string, error) {
	return p.ToJSONWithDebug(false)
}

// ToJSONWithDebug returns the Plan as structured JSON with optional source_diffs.
func (p *Plan) ToJSONWithDebug(includeSource bool) (string, error) {
	if !includeSource {
		for _, sp := range p.Schemas {
			sp.SourceDiffs = nil
		}
	}

	var buf strings.Builder
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(p); err != nil {
		return "", fmt.Errorf("failed to marshal plan to JSON: %w", err)
	}

	result := buf.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result, nil
}

// HumanColored returns a combined human-readable summary for all schemas.
// For single-schema plans it omits the schema header.
func (p *Plan) HumanColored(enableColor bool) string {
	names := p.SortedSchemaNames()
	if len(names) == 1 {
		return p.Schemas[names[0]].HumanColored(enableColor)
	}
	var out strings.Builder
	for _, name := range names {
		fmt.Fprintf(&out, "\n── Schema: %s ──────────────────────\n", name)
		out.WriteString(p.Schemas[name].HumanColored(enableColor))
	}
	return out.String()
}

// ToSQL returns combined SQL for all schemas.
// For single-schema plans it returns the SQL directly without a schema header.
func (p *Plan) ToSQL(format SQLFormat) string {
	names := p.SortedSchemaNames()
	if len(names) == 1 {
		return p.Schemas[names[0]].ToSQL(format)
	}
	var out strings.Builder
	for _, name := range names {
		sql := p.Schemas[name].ToSQL(format)
		if sql != "" {
			fmt.Fprintf(&out, "-- Schema: %s\n", name)
			out.WriteString(sql)
			if !strings.HasSuffix(sql, "\n") {
				out.WriteString("\n")
			}
			out.WriteString("\n")
		}
	}
	return out.String()
}

// FromJSON deserializes a Plan from JSON data.
func FromJSON(data []byte) (*Plan, error) {
	var raw struct {
		Version         string                     `json:"version"`
		PgschemaVersion string                     `json:"pgschema_version"`
		CreatedAt       time.Time                  `json:"created_at"`
		Schemas         map[string]json.RawMessage `json:"schemas"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan JSON: %w", err)
	}

	p := &Plan{
		Version:         raw.Version,
		PgschemaVersion: raw.PgschemaVersion,
		CreatedAt:       raw.CreatedAt,
		Schemas:         make(map[string]*SchemaPlan, len(raw.Schemas)),
	}

	for schemaName, schemaData := range raw.Schemas {
		var sp SchemaPlan
		if err := json.Unmarshal(schemaData, &sp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal plan for schema %s: %w", schemaName, err)
		}
		p.Schemas[schemaName] = &sp
	}

	return p, nil
}

// SummaryString returns a one-line summary of schemas and changes.
func (p *Plan) SummaryString() string {
	withChanges := 0
	for _, sp := range p.Schemas {
		if sp.HasAnyChanges() {
			withChanges++
		}
	}
	return fmt.Sprintf("Summary: %d schemas inspected, %d with changes", len(p.Schemas), withChanges)
}
