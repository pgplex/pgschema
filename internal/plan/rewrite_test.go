package plan

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// TestGenerateColumnNotNullRewrite verifies the version-gated SET NOT NULL
// rewrite: PostgreSQL 18+ uses the native NOT NULL ... NOT VALID constraint,
// while older (or unknown) versions use the portable four-step CHECK
// constraint pattern. The pre-18 path matters here because integration golden
// files only run against the latest PostgreSQL version.
func TestGenerateColumnNotNullRewrite(t *testing.T) {
	tests := []struct {
		name               string
		targetMajorVersion int
		wantSQL            []string
		wantIsolated       []bool
	}{
		{
			name:               "pg18 native not null not valid",
			targetMajorVersion: 18,
			wantSQL: []string{
				`ALTER TABLE users ADD CONSTRAINT users_email_not_null NOT NULL email NOT VALID;`,
				`ALTER TABLE users VALIDATE CONSTRAINT users_email_not_null;`,
			},
			wantIsolated: []bool{false, true},
		},
		{
			name:               "pre-18 check constraint pattern",
			targetMajorVersion: 17,
			wantSQL: []string{
				`ALTER TABLE users ADD CONSTRAINT email_not_null CHECK (email IS NOT NULL) NOT VALID;`,
				`ALTER TABLE users VALIDATE CONSTRAINT email_not_null;`,
				`ALTER TABLE users ALTER COLUMN email SET NOT NULL;`,
				`ALTER TABLE users DROP CONSTRAINT email_not_null;`,
			},
			wantIsolated: []bool{false, true, false, false},
		},
		{
			name:               "unknown version falls back to portable pattern",
			targetMajorVersion: 0,
			wantSQL: []string{
				`ALTER TABLE users ADD CONSTRAINT email_not_null CHECK (email IS NOT NULL) NOT VALID;`,
				`ALTER TABLE users VALIDATE CONSTRAINT email_not_null;`,
				`ALTER TABLE users ALTER COLUMN email SET NOT NULL;`,
				`ALTER TABLE users DROP CONSTRAINT email_not_null;`,
			},
			wantIsolated: []bool{false, true, false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := generateColumnNotNullRewrite(nil, "public.users.email", tt.targetMajorVersion, nil)

			if len(steps) != len(tt.wantSQL) {
				t.Fatalf("got %d steps, want %d:\n%s", len(steps), len(tt.wantSQL), stepsSQL(steps))
			}
			for i, step := range steps {
				if step.SQL != tt.wantSQL[i] {
					t.Errorf("step %d SQL:\ngot  %q\nwant %q", i, step.SQL, tt.wantSQL[i])
				}
				if step.RequiresIsolation != tt.wantIsolated[i] {
					t.Errorf("step %d RequiresIsolation = %v, want %v (%s)", i, step.RequiresIsolation, tt.wantIsolated[i], step.SQL)
				}
				if !step.CanRunInTransaction {
					t.Errorf("step %d should be able to run in a transaction (%s)", i, step.SQL)
				}
			}
		})
	}
}

// TestGenerateColumnNotNullRewriteQuoting verifies identifiers requiring
// quoting (camelCase, non-public schema) are quoted in the PG18 native path.
func TestGenerateColumnNotNullRewriteQuoting(t *testing.T) {
	steps := generateColumnNotNullRewrite(nil, "tenant.Users.createdAt", 18, nil)

	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2:\n%s", len(steps), stepsSQL(steps))
	}
	wantAdd := `ALTER TABLE tenant."Users" ADD CONSTRAINT "Users_createdAt_not_null" NOT NULL "createdAt" NOT VALID;`
	if steps[0].SQL != wantAdd {
		t.Errorf("add step SQL:\ngot  %q\nwant %q", steps[0].SQL, wantAdd)
	}
	wantValidate := `ALTER TABLE tenant."Users" VALIDATE CONSTRAINT "Users_createdAt_not_null";`
	if steps[1].SQL != wantValidate {
		t.Errorf("validate step SQL:\ngot  %q\nwant %q", steps[1].SQL, wantValidate)
	}
}

func stepsSQL(steps []RewriteStep) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteString(s.SQL)
		b.WriteString("\n")
	}
	return b.String()
}

// irWithConstraintNames builds a minimal current-state IR whose public.users
// table already has the given constraint names.
func irWithConstraintNames(names ...string) *ir.IR {
	taken := make(map[string]bool, len(names))
	for _, n := range names {
		taken[n] = true
	}
	return &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name: "public",
				Tables: map[string]*ir.Table{
					"users": {Schema: "public", Name: "users", AllConstraintNames: taken},
				},
			},
		},
	}
}

// TestGenerateColumnNotNullRewriteNameCollision verifies both rewrite paths
// pick a collision-free constraint name when the preferred name is already
// occupied on the table - typically by a leftover CHECK (col IS NOT NULL) from
// a manual online migration, which the inspector hides from the IR but which
// still makes ADD CONSTRAINT fail with SQLSTATE 42710 (greptile review on PR
// #566; same latent bug existed in the pre-18 path).
func TestGenerateColumnNotNullRewriteNameCollision(t *testing.T) {
	t.Run("pg18 native name occupied", func(t *testing.T) {
		currentIR := irWithConstraintNames("users_email_not_null")
		steps := generateColumnNotNullRewrite(nil, "public.users.email", 18, currentIR)

		wantAdd := `ALTER TABLE users ADD CONSTRAINT users_email_not_null1 NOT NULL email NOT VALID;`
		if len(steps) != 2 || steps[0].SQL != wantAdd {
			t.Fatalf("add step:\ngot  %q\nwant %q", steps[0].SQL, wantAdd)
		}
		wantValidate := `ALTER TABLE users VALIDATE CONSTRAINT users_email_not_null1;`
		if steps[1].SQL != wantValidate {
			t.Errorf("validate step:\ngot  %q\nwant %q", steps[1].SQL, wantValidate)
		}
	})

	t.Run("pg18 first suffix also occupied", func(t *testing.T) {
		currentIR := irWithConstraintNames("users_email_not_null", "users_email_not_null1")
		steps := generateColumnNotNullRewrite(nil, "public.users.email", 18, currentIR)

		wantAdd := `ALTER TABLE users ADD CONSTRAINT users_email_not_null2 NOT NULL email NOT VALID;`
		if len(steps) != 2 || steps[0].SQL != wantAdd {
			t.Fatalf("add step:\ngot  %q\nwant %q", steps[0].SQL, wantAdd)
		}
	})

	t.Run("pre-18 temp check name occupied", func(t *testing.T) {
		currentIR := irWithConstraintNames("email_not_null")
		steps := generateColumnNotNullRewrite(nil, "public.users.email", 17, currentIR)

		if len(steps) != 4 {
			t.Fatalf("got %d steps, want 4:\n%s", len(steps), stepsSQL(steps))
		}
		want := []string{
			`ALTER TABLE users ADD CONSTRAINT email_not_null1 CHECK (email IS NOT NULL) NOT VALID;`,
			`ALTER TABLE users VALIDATE CONSTRAINT email_not_null1;`,
			`ALTER TABLE users ALTER COLUMN email SET NOT NULL;`,
			`ALTER TABLE users DROP CONSTRAINT email_not_null1;`,
		}
		for i, w := range want {
			if steps[i].SQL != w {
				t.Errorf("step %d:\ngot  %q\nwant %q", i, steps[i].SQL, w)
			}
		}
	})

	t.Run("unoccupied name is used as-is", func(t *testing.T) {
		currentIR := irWithConstraintNames("some_other_constraint")
		steps := generateColumnNotNullRewrite(nil, "public.users.email", 18, currentIR)

		wantAdd := `ALTER TABLE users ADD CONSTRAINT users_email_not_null NOT NULL email NOT VALID;`
		if len(steps) != 2 || steps[0].SQL != wantAdd {
			t.Fatalf("add step:\ngot  %q\nwant %q", steps[0].SQL, wantAdd)
		}
	})
}
