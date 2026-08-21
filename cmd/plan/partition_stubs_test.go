package plan

import "testing"

func TestReplacePartitionOfRef(t *testing.T) {
	tests := []struct {
		name   string
		sql    string
		oldRef string
		newRef string
		want   string
	}{
		{
			name:   "simple unqualified rewrite",
			sql:    "CREATE TABLE child PARTITION OF public.parent FOR VALUES IN ('a');",
			oldRef: "public.parent",
			newRef: "parent",
			want:   "CREATE TABLE child PARTITION OF parent FOR VALUES IN ('a');",
		},
		{
			name:   "case insensitive keywords",
			sql:    "CREATE TABLE child partition of public.parent FOR VALUES IN ('a');",
			oldRef: "public.parent",
			newRef: "parent",
			want:   "CREATE TABLE child partition of parent FOR VALUES IN ('a');",
		},
		{
			name:   "multiple partitions same parent",
			sql:    "CREATE TABLE c1 PARTITION OF public.parent FOR VALUES IN ('a');\nCREATE TABLE c2 PARTITION OF public.parent FOR VALUES IN ('b');",
			oldRef: "public.parent",
			newRef: "parent",
			want:   "CREATE TABLE c1 PARTITION OF parent FOR VALUES IN ('a');\nCREATE TABLE c2 PARTITION OF parent FOR VALUES IN ('b');",
		},
		{
			name:   "partition by not affected",
			sql:    "CREATE TABLE parent (id int) PARTITION BY RANGE (id);",
			oldRef: "public.parent",
			newRef: "parent",
			want:   "CREATE TABLE parent (id int) PARTITION BY RANGE (id);",
		},
		{
			name:   "no match leaves sql unchanged",
			sql:    "CREATE TABLE child PARTITION OF other.parent FOR VALUES IN ('a');",
			oldRef: "public.parent",
			newRef: "parent",
			want:   "CREATE TABLE child PARTITION OF other.parent FOR VALUES IN ('a');",
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
