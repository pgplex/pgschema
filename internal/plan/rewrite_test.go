package plan

import (
	"strings"
	"testing"
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
			steps := generateColumnNotNullRewrite(nil, "public.users.email", tt.targetMajorVersion)

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
	steps := generateColumnNotNullRewrite(nil, "tenant.Users.createdAt", 18)

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
