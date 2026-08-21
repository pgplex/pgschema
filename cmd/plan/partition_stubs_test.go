package plan

import "testing"

func TestPartitionStubName(t *testing.T) {
	tests := []struct {
		schema, table, want string
	}{
		{"public", "events", "_pgschema_partstub_public__events"},
		{"archive", "events", "_pgschema_partstub_archive__events"},
	}
	for _, tt := range tests {
		got := partitionStubName(tt.schema, tt.table)
		if got != tt.want {
			t.Errorf("partitionStubName(%q, %q) = %q, want %q", tt.schema, tt.table, got, tt.want)
		}
	}
}

func TestReplacePartitionOfRef(t *testing.T) {
	tests := []struct {
		name   string
		sql    string
		oldRef string
		newRef string
		want   string
	}{
		{
			name:   "simple rewrite",
			sql:    "CREATE TABLE child PARTITION OF public.parent FOR VALUES IN ('a');",
			oldRef: "public.parent",
			newRef: `"_pgschema_partstub_public__parent"`,
			want:   `CREATE TABLE child PARTITION OF "_pgschema_partstub_public__parent" FOR VALUES IN ('a');`,
		},
		{
			name:   "case insensitive keywords",
			sql:    "CREATE TABLE child partition of public.parent FOR VALUES IN ('a');",
			oldRef: "public.parent",
			newRef: `"_pgschema_partstub_public__parent"`,
			want:   `CREATE TABLE child partition of "_pgschema_partstub_public__parent" FOR VALUES IN ('a');`,
		},
		{
			name:   "multiple partitions same parent",
			sql:    "CREATE TABLE c1 PARTITION OF public.parent FOR VALUES IN ('a');\nCREATE TABLE c2 PARTITION OF public.parent FOR VALUES IN ('b');",
			oldRef: "public.parent",
			newRef: `"_pgschema_partstub_public__parent"`,
			want:   "CREATE TABLE c1 PARTITION OF \"_pgschema_partstub_public__parent\" FOR VALUES IN ('a');\nCREATE TABLE c2 PARTITION OF \"_pgschema_partstub_public__parent\" FOR VALUES IN ('b');",
		},
		{
			name:   "partition by not affected",
			sql:    "CREATE TABLE parent (id int) PARTITION BY RANGE (id);",
			oldRef: "public.parent",
			newRef: `"_pgschema_partstub_public__parent"`,
			want:   "CREATE TABLE parent (id int) PARTITION BY RANGE (id);",
		},
		{
			name:   "no match leaves sql unchanged",
			sql:    "CREATE TABLE child PARTITION OF other.parent FOR VALUES IN ('a');",
			oldRef: "public.parent",
			newRef: `"_pgschema_partstub_public__parent"`,
			want:   "CREATE TABLE child PARTITION OF other.parent FOR VALUES IN ('a');",
		},
		{
			name:   "distinct schemas do not collide",
			sql:    "CREATE TABLE c1 PARTITION OF public.events FOR VALUES IN ('a');\nCREATE TABLE c2 PARTITION OF archive.events FOR VALUES IN ('b');",
			oldRef: "public.events",
			newRef: `"_pgschema_partstub_public__events"`,
			want:   "CREATE TABLE c1 PARTITION OF \"_pgschema_partstub_public__events\" FOR VALUES IN ('a');\nCREATE TABLE c2 PARTITION OF archive.events FOR VALUES IN ('b');",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replacePartitionOfRef(tt.sql, tt.oldRef, tt.newRef)
			if got != tt.want {
				t.Errorf("replacePartitionOfRef()\ngot:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}
