package diff

import "testing"

func TestExtractBaseTypeName(t *testing.T) {
	tests := []struct {
		name     string
		typeExpr string
		want     string
	}{
		{
			name:     "simple type",
			typeExpr: "integer",
			want:     "integer",
		},
		{
			name:     "SETOF simple type",
			typeExpr: "SETOF actor",
			want:     "actor",
		},
		{
			name:     "SETOF with schema",
			typeExpr: "SETOF public.actor",
			want:     "public.actor",
		},
		{
			name:     "SETOF with quoted type name",
			typeExpr: `SETOF public."ViewName"`,
			want:     "public.ViewName",
		},
		{
			name:     "SETOF with quoted schema and type",
			typeExpr: `SETOF "Schema"."ViewName"`,
			want:     "Schema.ViewName",
		},
		{
			name:     "quoted type only",
			typeExpr: `"MyType"`,
			want:     "MyType",
		},
		{
			name:     "array type",
			typeExpr: "integer[]",
			want:     "integer",
		},
		{
			name:     "SETOF quoted type with array",
			typeExpr: `SETOF public."ViewName"[]`,
			want:     "public.ViewName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBaseTypeName(tt.typeExpr)
			if got != tt.want {
				t.Errorf("extractBaseTypeName(%q) = %q, want %q", tt.typeExpr, got, tt.want)
			}
		})
	}
}
