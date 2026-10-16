package postgres

import (
	"strings"
	"testing"
)

func TestUnicodeInvalidUTF8(t *testing.T) {
	for _, body := range []string{
		"DO U&'BEGIN PERFORM ''public.t''::regclass; -- \xff\nEND';",
		"DO U&'BEGIN PERFORM ''public.t''::regclass; RAISE NOTICE ''\xc0\xaf''; END';",
		"DO $$ BEGIN PERFORM U&'public.t\xff'::regclass; END $$;",
		"DO U&'BEGIN -- \xc3'\n'\xa9\nEND';",
		"DO U&'BEGIN END' UESCAPE '\xff';",
		`DO U&'BEGIN END' UESCAPE E'\377';`,
	} {
		got, err := qualifyFunctionBodiesWithTempSchema(body, "public", "tmp", schemaRemapNames{})
		if err == nil || got != "" {
			t.Fatalf("invalid UTF-8 repaired: %q, %v", got, err)
		}
	}
	// A real replacement character is valid UTF-8 and must remain valid.
	tokens := lexSQL(`U&'�'`)
	if tokens[0].err != nil || tokens[0].content != "�" {
		t.Fatalf("valid replacement character rejected: %+v", tokens)
	}
}

func TestUnicodeContinuedTokens(t *testing.T) {
	for _, tc := range []struct{ sql, want string }{
		{"U&'a'\n'\\0062'", "ab"},
		{"U&'a'-- keep\n'\\0062'", "ab"},
		{"U&'a'\n-- keep\n'!0062' UESCAPE '!'", "ab"},
		{"U&'a'\r'\\0062'", "ab"},
		{"U&'\\D83D'\n'\\DE00'", "😀"},
		{"U&'\\00'\n'62'", "b"},
		{"U&'a' UESCAPE '!'\n''", "a"},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			tokens := lexSQL(tc.sql)
			if len(tokens) != 1 || tokens[0].err != nil || tokens[0].content != tc.want || tokens[0].text != tc.sql {
				t.Fatalf("tokens %+v; want %q", tokens, tc.want)
			}
		})
	}
	for _, gap := range []string{" ", " /*keep*/\n", "\n/*keep*/", "/*\n*/"} {
		sql := "U&'a'" + gap + "'b'"
		tokens := lexSQL(sql)
		if tokens[0].text != `U&'a'` || tokens[0].content != "a" {
			t.Fatalf("invalid continuation combined: %+v", tokens)
		}
	}
	for _, suffix := range []string{"'!'\n'?'", "E'!'\n'?'"} {
		if _, err := qualifyFunctionBodiesWithTempSchema("DO U&'BEGIN END' UESCAPE "+suffix, "public", "tmp", schemaRemapNames{}); err == nil {
			t.Fatal("continued multicharacter UESCAPE accepted")
		}
	}
}

func TestUnicodeContinuedDO(t *testing.T) {
	for _, sql := range []string{
		"DO U&'BEGIN PERFORM ''public.t''::regclass;'\n'\\0045ND';",
		"DO U&'BEGIN PERFORM ''public.t''::regclass;'\n'!0045ND' UESCAPE '!';",
		"DO U&'BEGIN'-- keep\n' PERFORM ''public.t''::regclass; END';",
		"DO $$ BEGIN PERFORM U&'public.'\n'!0074' UESCAPE '!'::regclass; END $$;",
	} {
		got, err := qualifyFunctionBodiesWithTempSchema(sql, "public", "tmp", schemaRemapNames{})
		if err != nil || !strings.Contains(got, `"tmp".t`) || !strings.Contains(got, "END") {
			t.Fatalf("continued DO not remapped: %s, %v", got, err)
		}
		if strings.Contains(sql, "-- keep") && !strings.Contains(got, "-- keep\n") {
			t.Fatalf("lost continuation comment: %s", got)
		}
	}
	const unchanged = "DO U&'BEGIN'-- keep\n' RAISE NOTICE ''public.t''; END' UESCAPE '!';"
	got, err := qualifyFunctionBodiesWithTempSchema(unchanged, "public", "tmp", schemaRemapNames{})
	if err != nil || got != unchanged {
		t.Fatalf("unchanged spelling lost: %s, %v", got, err)
	}
}

func TestRegclassNamesAreNotSQL(t *testing.T) {
	for _, name := range []string{`U&"public".t`, `public.U&"t"`, `other.t`, `public."t"x`, `public./*comment*/t`} {
		sql := `DO $$ BEGIN PERFORM '` + name + `'::regclass; END $$;`
		got, err := qualifyFunctionBodiesWithTempSchema(sql, "public", "tmp", schemaRemapNames{relations: map[string]bool{"t": true}})
		if err != nil || got != sql {
			t.Fatalf("name interpreted as SQL: %s, %v", got, err)
		}
	}
	for _, name := range []string{`public.té`, `public.t!`, `public.t/*x*/`, `public.t\x`, `public."t""q"`, ` public . "té" `} {
		sql := `DO $$ BEGIN PERFORM '` + name + `'::regclass; END $$;`
		got, err := qualifyFunctionBodiesWithTempSchema(sql, "public", "tmp", schemaRemapNames{})
		want := strings.Replace(sql, "public", `"tmp"`, 1)
		if err != nil || got != want {
			t.Fatalf("unusual name lost: %s, %v; want %s", got, err, want)
		}
	}
}

func TestUnicodeEncodingRetainsSemantics(t *testing.T) {
	sql := `DO $$ BEGIN SET standard_conforming_strings=off; PERFORM U&'public.t'::regclass; END $$;`
	got, err := qualifyFunctionBodiesWithTempSchema(sql, "public", "tmp", schemaRemapNames{})
	if err != nil || !strings.Contains(got, `U&'"tmp".t'`) {
		t.Fatalf("Unicode semantics lost: %s, %v", got, err)
	}
	for _, raw := range []string{`u&'!0070ublic."t!!\x''q"' /* keep */ UESCAPE '!'`, `U&'public."t\\x''q"'`} {
		token := lexSQL(raw)[0]
		if token.err != nil {
			t.Fatal(token.err)
		}
		encoded := encodeSQLLiteral(token, token.content)
		decoded := lexSQL(encoded)[0]
		if decoded.err != nil || decoded.content != token.content || !decoded.unicode {
			t.Fatalf("Unicode round trip %q => %q => %+v", raw, encoded, decoded)
		}
		if strings.Contains(raw, "/* keep */") && !strings.Contains(encoded, "/* keep */") {
			t.Fatal("lost comment")
		}
	}
}
