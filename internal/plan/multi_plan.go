package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pgplex/pgschema/internal/color"
	"github.com/pgplex/pgschema/internal/version"
)

// SchemaEntry holds plan data for a single schema within a MultiPlan.
// It excludes top-level metadata (version, pgschema_version, created_at)
// which live on the MultiPlan itself.
type SchemaEntry struct {
	*Plan
}

// MarshalJSON serializes a SchemaEntry, excluding top-level metadata fields
// that are already present on the parent MultiPlan.
func (e SchemaEntry) MarshalJSON() ([]byte, error) {
	type entry struct {
		SourceFingerprint any              `json:"source_fingerprint,omitempty"`
		Groups            []ExecutionGroup `json:"groups"`
		SourceDiffs       any              `json:"source_diffs,omitempty"`
	}
	out := entry{
		Groups: e.Plan.Groups,
	}
	if e.Plan.SourceFingerprint != nil {
		out.SourceFingerprint = e.Plan.SourceFingerprint
	}
	if len(e.Plan.SourceDiffs) > 0 {
		out.SourceDiffs = e.Plan.SourceDiffs
	}
	return json.Marshal(out)
}

// MultiPlan holds migration plans for multiple schemas in a single file.
type MultiPlan struct {
	Version         string                  `json:"version"`
	PgschemaVersion string                  `json:"pgschema_version"`
	CreatedAt       time.Time               `json:"created_at"`
	Schemas         map[string]*SchemaEntry `json:"schemas"`
}

// NewMultiPlan creates a new MultiPlan with version metadata and current timestamp.
func NewMultiPlan() *MultiPlan {
	createdAt := time.Now().Truncate(time.Second)
	if testTime := os.Getenv("PGSCHEMA_TEST_TIME"); testTime != "" {
		if parsedTime, err := time.Parse(time.RFC3339, testTime); err == nil {
			createdAt = parsedTime
		}
	}
	return &MultiPlan{
		Version:         version.PlanFormat(),
		PgschemaVersion: version.App(),
		CreatedAt:       createdAt,
		Schemas:         make(map[string]*SchemaEntry),
	}
}

// AddSchema adds a per-schema plan to the MultiPlan.
func (mp *MultiPlan) AddSchema(schemaName string, p *Plan) {
	mp.Schemas[schemaName] = &SchemaEntry{Plan: p}
}

// HasAnyChanges returns true if any schema plan has changes.
func (mp *MultiPlan) HasAnyChanges() bool {
	for _, entry := range mp.Schemas {
		if entry.Plan.HasAnyChanges() {
			return true
		}
	}
	return false
}

// SortedSchemaNames returns schema names in sorted order for deterministic iteration.
func (mp *MultiPlan) SortedSchemaNames() []string {
	names := make([]string, 0, len(mp.Schemas))
	for name := range mp.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ToJSON returns the MultiPlan as structured JSON.
func (mp *MultiPlan) ToJSON() (string, error) {
	return mp.ToJSONWithDebug(false)
}

// ToJSONWithDebug returns the MultiPlan as structured JSON with optional source_diffs.
func (mp *MultiPlan) ToJSONWithDebug(includeSource bool) (string, error) {
	// If not including source, strip source_diffs from all schema entries before marshaling.
	if !includeSource {
		for _, entry := range mp.Schemas {
			entry.Plan.SourceDiffs = nil
		}
	}

	var buf strings.Builder
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(mp); err != nil {
		return "", fmt.Errorf("failed to marshal multi-plan to JSON: %w", err)
	}

	result := buf.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result, nil
}

// HumanColored returns a combined human-readable summary for all schemas.
func (mp *MultiPlan) HumanColored(enableColor bool) string {
	var out strings.Builder
	for _, name := range mp.SortedSchemaNames() {
		entry := mp.Schemas[name]
		fmt.Fprintf(&out, "\n── Schema: %s ──────────────────────\n", name)
		out.WriteString(entry.Plan.HumanColored(enableColor))
	}
	return out.String()
}

// ToSQL returns combined SQL for all schemas with schema headers.
func (mp *MultiPlan) ToSQL(format SQLFormat) string {
	var out strings.Builder
	for _, name := range mp.SortedSchemaNames() {
		entry := mp.Schemas[name]
		sql := entry.Plan.ToSQL(format)
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

// multiPlanDetect is used for JSON format auto-detection.
type multiPlanDetect struct {
	Schemas json.RawMessage `json:"schemas"`
	Groups  json.RawMessage `json:"groups"`
}

// LoadPlanFile reads a plan JSON file and returns an Outputter.
// It auto-detects the format by checking for the "schemas" key (MultiPlan)
// vs "groups" key (single Plan).
func LoadPlanFile(data []byte) (Outputter, error) {
	var detect multiPlanDetect
	if err := json.Unmarshal(data, &detect); err != nil {
		return nil, fmt.Errorf("failed to parse plan file: %w", err)
	}

	if detect.Schemas != nil {
		// MultiPlan format
		mp, err := MultiPlanFromJSON(data)
		if err != nil {
			return nil, err
		}
		return mp, nil
	}

	// Single Plan format
	p, err := FromJSON(data)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// MultiPlanFromJSON deserializes a MultiPlan from JSON data.
func MultiPlanFromJSON(data []byte) (*MultiPlan, error) {
	// First unmarshal the wrapper with raw schema entries.
	var raw struct {
		Version         string                     `json:"version"`
		PgschemaVersion string                     `json:"pgschema_version"`
		CreatedAt       time.Time                  `json:"created_at"`
		Schemas         map[string]json.RawMessage `json:"schemas"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal multi-plan JSON: %w", err)
	}

	mp := &MultiPlan{
		Version:         raw.Version,
		PgschemaVersion: raw.PgschemaVersion,
		CreatedAt:       raw.CreatedAt,
		Schemas:         make(map[string]*SchemaEntry, len(raw.Schemas)),
	}

	for schemaName, schemaData := range raw.Schemas {
		var p Plan
		if err := json.Unmarshal(schemaData, &p); err != nil {
			return nil, fmt.Errorf("failed to unmarshal plan for schema %s: %w", schemaName, err)
		}
		// Populate top-level fields from the parent.
		p.Version = raw.Version
		p.PgschemaVersion = raw.PgschemaVersion
		p.CreatedAt = raw.CreatedAt
		mp.Schemas[schemaName] = &SchemaEntry{Plan: &p}
	}

	return mp, nil
}


// SummaryString returns a one-line summary of schemas and changes.
func (mp *MultiPlan) SummaryString() string {
	withChanges := 0
	for _, entry := range mp.Schemas {
		if entry.Plan.HasAnyChanges() {
			withChanges++
		}
	}
	return fmt.Sprintf("Summary: %d schemas inspected, %d with changes", len(mp.Schemas), withChanges)
}

// DisplayMultiPlanForApply prints the combined human output for all schemas in a MultiPlan.
func DisplayMultiPlanForApply(mp *MultiPlan, noColor bool) {
	c := color.New(!noColor)
	for _, name := range mp.SortedSchemaNames() {
		entry := mp.Schemas[name]
		fmt.Fprintf(os.Stderr, "\n%s\n", c.Bold(fmt.Sprintf("── Schema: %s ──────────────────────", name)))
		fmt.Print(entry.Plan.HumanColored(!noColor))
	}
}
