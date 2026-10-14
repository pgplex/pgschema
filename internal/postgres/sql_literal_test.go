package postgres

import (
	"strings"
	"testing"
)

func TestSQLLiteralUnicodeSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, literal, want string
		invalid             bool
	}{
		{name: "short_u", literal: `E'\u12'`, invalid: true},
		{name: "short_U", literal: `E'\U0001F60'`, invalid: true},
		{name: "invalid_hex", literal: `E'\u12GG'`, invalid: true},
		{name: "invalid_sign", literal: `E'\u+001'`, invalid: true},
		{name: "out_of_range", literal: `E'\U00110000'`, invalid: true},
		{name: "maximum_uint32", literal: `E'\UFFFFFFFF'`, invalid: true},
		{name: "zero", literal: `E'\u0000'`, invalid: true},
		{name: "unpaired_high", literal: `E'\uD83D'`, invalid: true},
		{name: "unpaired_low", literal: `E'\uDE00'`, invalid: true},
		{name: "wrong_second_surrogate", literal: `E'\uD83D\u0041'`, invalid: true},
		{name: "incomplete_second_surrogate", literal: `E'\uD83D\uDE'`, invalid: true},
		{name: "surrogate_pair", literal: `E'\uD83D\uDE00'`, want: "😀"},
		{name: "mixed_width_surrogate_pair", literal: `E'\U0000D83D\uDE00'`, want: "😀"},
		{name: "wide_low_surrogate", literal: `E'\uD83D\U0000DE00'`, want: "😀"},
		{name: "single_code_point", literal: `E'\U0001F600'`, want: "😀"},
		{name: "bmp", literal: `E'\u00E9'`, want: "é"},
		{name: "escaped_backslash", literal: `E'\\u12'`, want: `\u12`},
		{name: "escaped_quote", literal: `E'it\'s'`, want: "it's"},
		{name: "standard_literal", literal: `'\u12'`, want: `\u12`},
		{name: "doubled_quote", literal: `E'it''s'`, want: "it's"},
		{name: "octal_wrap", literal: `E'\777'`, want: string([]byte{255})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokens := lexSQL(tc.literal)
			if len(tokens) != 1 || !completeSQLLiteral(tokens[0]) {
				t.Fatalf("invalid test literal %s", tc.literal)
			}
			got, err := literalContent(tokens[0])
			if tc.invalid {
				if err == nil || got != "" {
					t.Fatalf("invalid literal decoded to %q, error %v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestRemapDOBodiesRejectsInvalidEscapeLiterals(t *testing.T) {
	for _, sql := range []string{
		`DO E'BEGIN RAISE NOTICE ''\u12''; END';`,
		`DO E'BEGIN RAISE NOTICE ''\U00110000''; END';`,
		`DO E'BEGIN RAISE NOTICE ''\uD83D''; END';`,
		`DO $$ BEGIN PERFORM E'public.\u12'::pg_catalog.regclass; END $$;`,
	} {
		got, err := qualifyFunctionBodiesWithTempSchema(sql, "public", "pgschema_tmp_x", nil)
		if err == nil || got != "" {
			t.Fatalf("invalid DO literal was repaired to %q, error %v", got, err)
		}
		if !strings.Contains(err.Error(), "DO body") {
			t.Fatalf("missing remapping context: %v", err)
		}
	}
}

func TestRemapDOSurrogatePairs(t *testing.T) {
	const original = `DO E'BEGIN PERFORM ''public.t''::pg_catalog.regclass; RAISE NOTICE ''\uD83D\uDE00''; END';`
	got, err := qualifyFunctionBodiesWithTempSchema(original, "public", "pgschema_tmp_x", nil)
	if err != nil {
		t.Fatal(err)
	}
	const want = `DO E'BEGIN PERFORM ''"pgschema_tmp_x".t''::pg_catalog.regclass; RAISE NOTICE ''😀''; END';`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	const untouched = `DO E'BEGIN RAISE NOTICE ''\uD83D\uDE00''; END';`
	got, err = qualifyFunctionBodiesWithTempSchema(untouched, "public", "pgschema_tmp_x", nil)
	if err != nil || got != untouched {
		t.Fatalf("unchanged body re-encoded: %s, %v", got, err)
	}
}
