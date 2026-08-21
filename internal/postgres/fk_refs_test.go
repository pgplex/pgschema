package postgres

import (
	"reflect"
	"testing"
)

func TestExtractForeignKeyTargets(t *testing.T) {
	tests := []struct {
		name          string
		sql           string
		defaultSchema string
		want          []QualifiedName
	}{
		{
			name:          "column-level cross-schema",
			sql:           "CREATE TABLE users (id uuid, auth_user_id uuid REFERENCES auth.users (id) ON DELETE CASCADE);",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "auth", Table: "users"}},
		},
		{
			name:          "table-level constraint",
			sql:           "CREATE TABLE profiles (user_id uuid, CONSTRAINT fk_auth FOREIGN KEY (user_id) REFERENCES auth.users (id));",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "auth", Table: "users"}},
		},
		{
			name:          "unqualified uses default schema",
			sql:           "CREATE TABLE orders (user_id int REFERENCES users(id));",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "public", Table: "users"}},
		},
		{
			name:          "table name can be public",
			sql:           "CREATE TABLE child (ref_id int REFERENCES public(id));",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "public", Table: "public"}},
		},
		{
			name:          "quoted identifiers",
			sql:           `CREATE TABLE t (id int REFERENCES "Auth"."Users" (id));`,
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "Auth", Table: "Users"}},
		},
		{
			name:          "skips string literals",
			sql:           "CREATE TABLE t (id int, note text DEFAULT 'REFERENCES auth.users (id)');",
			defaultSchema: "public",
			want:          nil,
		},
		{
			name:          "skips comments",
			sql:           "CREATE TABLE t (id int); -- REFERENCES auth.users (id)",
			defaultSchema: "public",
			want:          nil,
		},
		{
			name:          "skips dollar-quoted bodies",
			sql:           "CREATE FUNCTION f() RETURNS void AS $$ BEGIN PERFORM REFERENCES auth.users; END; $$ LANGUAGE plpgsql;",
			defaultSchema: "public",
			want:          nil,
		},
		{
			name:          "skips GRANT REFERENCES ON",
			sql:           "GRANT REFERENCES ON TABLE users TO app;",
			defaultSchema: "public",
			want:          nil,
		},
		{
			name:          "deduplicates",
			sql:           "CREATE TABLE a (x uuid REFERENCES auth.users(id)); CREATE TABLE b (y uuid REFERENCES auth.users(id));",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "auth", Table: "users"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractForeignKeyTargets(tt.sql, tt.defaultSchema)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractForeignKeyTargets() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExtractCreateTableNames(t *testing.T) {
	tests := []struct {
		name          string
		sql           string
		defaultSchema string
		want          []QualifiedName
	}{
		{
			name:          "simple",
			sql:           "CREATE TABLE users (id int);",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "public", Table: "users"}},
		},
		{
			name:          "if not exists qualified",
			sql:           "CREATE TABLE IF NOT EXISTS auth.users (id uuid PRIMARY KEY);",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "auth", Table: "users"}},
		},
		{
			name:          "unlogged",
			sql:           "CREATE UNLOGGED TABLE cache (k text);",
			defaultSchema: "backend",
			want:          []QualifiedName{{Schema: "backend", Table: "cache"}},
		},
		{
			name:          "skips create schema",
			sql:           "CREATE SCHEMA auth; CREATE TABLE auth.users (id uuid);",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "auth", Table: "users"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCreateTableNames(tt.sql, tt.defaultSchema)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractCreateTableNames() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExtractPartitionOfTargets(t *testing.T) {
	tests := []struct {
		name          string
		sql           string
		defaultSchema string
		want          []QualifiedName
	}{
		{
			name:          "simple partition of",
			sql:           "CREATE TABLE child PARTITION OF parent FOR VALUES IN ('a');",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "public", Table: "parent"}},
		},
		{
			name:          "qualified partition of",
			sql:           "CREATE TABLE data.child PARTITION OF core.parent FOR VALUES FROM (1) TO (100);",
			defaultSchema: "data",
			want:          []QualifiedName{{Schema: "core", Table: "parent"}},
		},
		{
			name:          "multiple partitions same parent deduped",
			sql:           "CREATE TABLE c1 PARTITION OF parent FOR VALUES IN ('a');\nCREATE TABLE c2 PARTITION OF parent FOR VALUES IN ('b');",
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "public", Table: "parent"}},
		},
		{
			name:          "partition by not matched",
			sql:           "CREATE TABLE parent (id int) PARTITION BY RANGE (id);",
			defaultSchema: "public",
			want:          nil,
		},
		{
			name:          "quoted identifiers",
			sql:           `CREATE TABLE "Child" PARTITION OF "Parent" FOR VALUES IN (1);`,
			defaultSchema: "public",
			want:          []QualifiedName{{Schema: "public", Table: "Parent"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPartitionOfTargets(tt.sql, tt.defaultSchema)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractPartitionOfTargets() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
