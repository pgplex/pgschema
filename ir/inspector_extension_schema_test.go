package ir

import "testing"

// stripSameSchemaPrefix must strip a prefix matching either the routine's own
// schema, or (only for the schema this Inspector's managedSchema is set to,
// and only for a catalog-verified extension member of it) an
// extension-owned type (issue #518): when introspecting pgschema's own
// temporary comparison schema, every function's routineSchema is that temp
// schema's literal name regardless of which real schema it represents, so an
// extension-owned type's real schema qualifier (e.g. "domain.vector") never
// matches routineSchema and would otherwise survive unstripped - causing the
// temp-schema side and the real-target side of a diff to compare as
// different function signatures.
//
// Scoping the fallback to managedSchema specifically (never "any known
// extension schema") matters because a function that is itself part of the
// schema being managed can still take a parameter whose type genuinely lives
// in a different schema (e.g. a function declared in a managed "app" schema
// taking a "domain.vector" parameter, where pgvector lives in "domain", not
// "app"). Since every function's routineSchema during temp-schema
// introspection is the same temp-schema literal regardless of which real
// schema it represents, a check keyed off "is routineSchema a temp schema"
// alone can't distinguish that genuine cross-schema reference from a
// same-managed-schema one - only checking against managedSchema itself can
// (a regression caught in PR #608 review, HIGH severity: the original
// temp-schema-prefix-only gate stripped this case incorrectly).
//
// It also must NOT fire for a type that merely lives in managedSchema
// without being an extension member (a schema can host both extension-owned
// and ordinary user-defined objects) - another regression caught in PR #608
// review, addressed by checking extensionOwnedTypes rather than
// extensionSchemas alone.
func TestStripSameSchemaPrefix_ExtensionSchemaAware(t *testing.T) {
	tests := []struct {
		name                string
		typeName            string
		routineSchema       string
		managedSchema       string
		extensionSchemas    map[string]bool
		extensionOwnedTypes map[string]bool
		want                string
	}{
		{
			name:          "strips routine's own schema, no extension schemas known",
			typeName:      "domain.mytype",
			routineSchema: "domain",
			managedSchema: "domain",
			want:          "mytype",
		},
		{
			name:                "temp-schema introspection, type is a confirmed member of managedSchema",
			typeName:            "domain.vector",
			routineSchema:       "pgschema_tmp_20260101_000000_abcd1234",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "vector",
		},
		{
			name:                "quoted extension schema qualifier",
			typeName:            `"domain".vector`,
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "vector",
		},
		{
			name:                "array of a confirmed extension member type",
			typeName:            "domain.vector[]",
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "vector[]",
		},
		{
			name:                "quoted mixed-case extension member type - membership check must unquote",
			typeName:            `domain."Vector"`,
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.Vector": true},
			want:                `"Vector"`,
		},
		{
			name:                "quoted mixed-case array of a confirmed extension member type",
			typeName:            `domain."Vector"[]`,
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.Vector": true},
			want:                `"Vector"[]`,
		},
		{
			// A function declared in the managed "app" schema takes a parameter
			// whose type genuinely lives in "domain" (a different schema, which
			// hosts pgvector) - real-target-side introspection: routineSchema
			// equals managedSchema ("app"), which does not itself host any
			// extension, so the fallback never even considers "domain".
			name:                "genuine cross-schema reference within the managed schema's own function - real side",
			typeName:            "domain.vector",
			routineSchema:       "app",
			managedSchema:       "app",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "domain.vector",
		},
		{
			// Same scenario, but introspected via the temp schema - HIGH
			// severity regression from PR #608 review: a gate based only on
			// "is routineSchema a temp schema" would strip this incorrectly,
			// since every function's routineSchema is the same temp-schema
			// literal here regardless of which real schema it belongs to.
			// Checking against managedSchema ("app", which hosts no
			// extension) rather than looping every known extension schema is
			// what keeps this qualified, matching the real side above.
			name:                "genuine cross-schema reference within the managed schema's own function - temp side",
			typeName:            "domain.vector",
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "app",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "domain.vector",
		},
		{
			name:                "managed schema hosts an extension, but this specific type is not a member",
			typeName:            "exts.status",
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "exts",
			extensionSchemas:    map[string]bool{"exts": true},
			extensionOwnedTypes: map[string]bool{"exts.vector": true},
			want:                "exts.status",
		},
		{
			name:                "cross-schema type unaffected - schema matches neither routine nor managed schema",
			typeName:            "utils.hstore",
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "utils.hstore",
		},
		{
			name:                "already-bare type unaffected",
			typeName:            "vector",
			routineSchema:       "pgschema_tmp_xxx",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "vector",
		},
		{
			name:                "empty type name",
			typeName:            "",
			routineSchema:       "domain",
			managedSchema:       "domain",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
			want:                "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insp := &Inspector{managedSchema: tt.managedSchema, extensionSchemas: tt.extensionSchemas, extensionOwnedTypes: tt.extensionOwnedTypes}
			if got := insp.stripSameSchemaPrefix(tt.typeName, tt.routineSchema); got != tt.want {
				t.Errorf("stripSameSchemaPrefix(%q, %q) = %q, want %q", tt.typeName, tt.routineSchema, got, tt.want)
			}
		})
	}
}

// stripSameSchemaPrefixFromReturnType must decompose SETOF and TABLE(...)
// return types the same way ir/normalize.go's stripSchemaFromReturnType
// does, applying the managedSchema-scoped stripSameSchemaPrefix to each
// contained type rather than a single top-level prefix check. Without this,
// "RETURNS vector" would compare as "domain.vector" (temp side) vs "vector"
// (real target side) and spuriously trigger a drop+recreate (PR #608 review
// feedback).
func TestStripSameSchemaPrefixFromReturnType(t *testing.T) {
	insp := &Inspector{
		managedSchema:       "domain",
		extensionSchemas:    map[string]bool{"domain": true},
		extensionOwnedTypes: map[string]bool{"domain.vector": true},
	}

	tests := []struct {
		name          string
		returnType    string
		routineSchema string
		want          string
	}{
		{
			name:          "direct extension-owned return type in temp schema",
			returnType:    "domain.vector",
			routineSchema: "pgschema_tmp_xxx",
			want:          "vector",
		},
		{
			name:          "SETOF extension-owned return type in temp schema",
			returnType:    "SETOF domain.vector",
			routineSchema: "pgschema_tmp_xxx",
			want:          "SETOF vector",
		},
		{
			name:          "TABLE(...) column referencing an extension-owned type in temp schema",
			returnType:    "TABLE(id integer, embedding domain.vector)",
			routineSchema: "pgschema_tmp_xxx",
			want:          "TABLE(id integer, embedding vector)",
		},
		{
			name:          "direct extension-owned return type on the managed schema's own real routine",
			returnType:    "domain.vector",
			routineSchema: "domain",
			want:          "vector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := insp.stripSameSchemaPrefixFromReturnType(tt.returnType, tt.routineSchema); got != tt.want {
				t.Errorf("stripSameSchemaPrefixFromReturnType(%q, %q) = %q, want %q", tt.returnType, tt.routineSchema, got, tt.want)
			}
		})
	}

	t.Run("genuine cross-schema return type on a differently-managed schema is preserved", func(t *testing.T) {
		appInsp := &Inspector{
			managedSchema:       "app",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
		}
		for _, routineSchema := range []string{"app", "pgschema_tmp_xxx"} {
			if got := appInsp.stripSameSchemaPrefixFromReturnType("domain.vector", routineSchema); got != "domain.vector" {
				t.Errorf("stripSameSchemaPrefixFromReturnType(%q, %q) = %q, want %q", "domain.vector", routineSchema, got, "domain.vector")
			}
		}
	})
}

// stripExtensionMemberTypeQualifiers is buildPrivileges' equivalent of
// stripSameSchemaPrefix's extension-membership check, operating on a whole
// function/procedure identity-arguments string instead of a single type, and
// scoped to managedSchema the same way.
func TestStripExtensionMemberTypeQualifiers(t *testing.T) {
	insp := &Inspector{
		managedSchema:       "domain",
		extensionSchemas:    map[string]bool{"domain": true},
		extensionOwnedTypes: map[string]bool{"domain.vector": true, "domain.Vector": true},
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips a confirmed extension member type in a signature",
			in:   "vector_search(query_embedding domain.vector)",
			want: "vector_search(query_embedding vector)",
		},
		{
			name: "preserves a non-member type in the same extension schema",
			in:   "f(x domain.status)",
			want: "f(x domain.status)",
		},
		{
			name: "leaves an unrelated schema untouched",
			in:   "g(x utils.hstore)",
			want: "g(x utils.hstore)",
		},
		{
			name: "strips a quoted mixed-case confirmed extension member type",
			in:   `h(x domain."Vector")`,
			want: `h(x "Vector")`,
		},
		{
			// A parameter literally NAMED the quoted identifier "domain.vector"
			// (a valid, if unusual, Postgres identifier - dots are permitted
			// inside quotes). A raw-text regex can't tell this apart from a
			// genuine schema.type qualifier since it doesn't track quoting
			// context; the tokenizer treats the whole quoted string as one
			// atomic token and never looks inside it (PR #608 review feedback).
			name: "quoted identifier that merely contains dotted text is left untouched",
			in:   `f("domain.vector" integer)`,
			want: `f("domain.vector" integer)`,
		},
		{
			// A quoted schema qualifier - the old regex only ever matched the
			// bare, unquoted schema name literally, so this never stripped at
			// all (PR #608 review feedback).
			name: "quoted schema qualifier on a confirmed member type",
			in:   `k(x "domain".vector)`,
			want: `k(x vector)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := insp.stripExtensionMemberTypeQualifiers(tt.in); got != tt.want {
				t.Errorf("stripExtensionMemberTypeQualifiers(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	t.Run("genuine cross-schema privilege signature on a differently-managed schema is preserved", func(t *testing.T) {
		appInsp := &Inspector{
			managedSchema:       "app",
			extensionSchemas:    map[string]bool{"domain": true},
			extensionOwnedTypes: map[string]bool{"domain.vector": true},
		}
		in := "vector_search(query_embedding domain.vector)"
		if got := appInsp.stripExtensionMemberTypeQualifiers(in); got != in {
			t.Errorf("stripExtensionMemberTypeQualifiers(%q) = %q, want unchanged %q", in, got, in)
		}
	})
}

// stripSameSchemaPrefixFromList (used for aggregate identity args and
// signatures) must also apply managedSchema's extension-membership-aware
// stripping, not just the basic same-schema strip - otherwise an aggregate
// over an extension-owned type keys as "vector" on the real side but stays
// "domain.vector" on the temp side and is spuriously dropped/recreated (PR
// #608 review feedback).
func TestStripSameSchemaPrefixFromList(t *testing.T) {
	insp := &Inspector{
		managedSchema:       "domain",
		extensionSchemas:    map[string]bool{"domain": true},
		extensionOwnedTypes: map[string]bool{"domain.vector": true},
	}
	in := "domain.vector"
	want := "vector"
	if got := insp.stripSameSchemaPrefixFromList(in, "pgschema_tmp_xxx"); got != want {
		t.Errorf("stripSameSchemaPrefixFromList(%q, %q) = %q, want %q", in, "pgschema_tmp_xxx", got, want)
	}
}
