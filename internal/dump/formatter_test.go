package dump

import "testing"

// TestGetCommentSchemaName_QualifySchema covers the comment-header schema label,
// which is observable output for `dump --qualify-schema`. Without the flag the
// target schema collapses to "-" (pg_dump style); with the flag it is preserved
// so the header matches the schema-qualified DDL. Cross-schema paths always keep
// their schema; unqualified paths are returned unchanged.
func TestGetCommentSchemaName_QualifySchema(t *testing.T) {
	tests := []struct {
		name          string
		targetSchema  string
		qualifySchema bool
		path          string
		want          string
	}{
		{
			name:          "target schema collapses to dash by default",
			targetSchema:  "public",
			qualifySchema: false,
			path:          "public.users",
			want:          "-",
		},
		{
			name:          "target schema is preserved under forced qualification",
			targetSchema:  "public",
			qualifySchema: true,
			path:          "public.users",
			want:          "public",
		},
		{
			name:          "cross-schema path keeps its schema (default)",
			targetSchema:  "public",
			qualifySchema: false,
			path:          "other.users",
			want:          "other",
		},
		{
			name:          "cross-schema path keeps its schema (forced)",
			targetSchema:  "public",
			qualifySchema: true,
			path:          "other.users",
			want:          "other",
		},
		{
			name:          "unqualified path is returned unchanged",
			targetSchema:  "public",
			qualifySchema: true,
			path:          "users",
			want:          "users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDumpFormatter("PostgreSQL 17.5", tt.targetSchema, false, tt.qualifySchema)
			if got := f.getCommentSchemaName(tt.path); got != tt.want {
				t.Errorf("getCommentSchemaName(%q) with qualifySchema=%v = %q, want %q",
					tt.path, tt.qualifySchema, got, tt.want)
			}
		})
	}
}
