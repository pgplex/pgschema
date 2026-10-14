package postgres

import (
	"reflect"
	"strings"
	"testing"
)

func TestSQLUnicodeTokens(t *testing.T) {
	for _, tc := range []struct{ sql, want string }{
		{`U&'\0070ublic.\+01F600'`, "public.😀"},
		{`u&"\0070ublic"`, "public"},
		{`U&'\D83D\DE00'`, "😀"},
		{`U&'\D83D\+00DE00'`, "😀"},
		{`U&'it''s \\ text'`, "it's \\ text"},
		{`U&"a""b"`, `a"b`},
		{`u&'!0070ublic!!t' UESCAPE '!'`, "public!t"},
		{`U&"!0070ublic" /* keep */ UESCAPE /* also */ E'!'`, "public"},
		{`U&'!0061' UESCAPE $$!$$`, "a"},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			tokens := lexSQL(tc.sql)
			if len(tokens) != 1 || tokens[0].err != nil {
				t.Fatalf("tokens %+v", tokens)
			}
			token := tokens[0]
			got := tokenIdentifier(token)
			if token.kind == 's' {
				var err error
				got, err = literalContent(token)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got != tc.want || token.text != tc.sql {
				t.Fatalf("got %q, raw %q; want %q, raw %q", got, token.text, tc.want, tc.sql)
			}
		})
	}
}

func TestSQLUnicodeInvalid(t *testing.T) {
	for _, sql := range []string{
		`U&'\12'`, `U&'\12GG'`, `U&'\+110000'`, `U&'\0000'`,
		`U&'\D83D'`, `U&'\DE00'`, `U&'\D83D\0041'`, `U&'\D83D\DE'`,
		`U&'a' UESCAPE ''`, `U&'a' UESCAPE 'ab'`, `U&'a' UESCAPE '+'`,
		`U&'a' UESCAPE '0'`, `U&'a' UESCAPE 'F'`, `U&'a' UESCAPE ' '`,
		`U&'a' UESCAPE '"'`, `U&'a' UESCAPE ''''`, `U&'a' UESCAPE '§'`,
		`U&'a' UESCAPE U&'!'`, `U&'a' UESCAPE`, `U&'a' UESCAPE E'\u12'`,
		`U&'unfinished`, `U&"unfinished`,
	} {
		t.Run(sql, func(t *testing.T) {
			if _, err := stripSchemaQualifications("SELECT "+sql, "public"); err == nil {
				t.Fatal("strip accepted invalid Unicode")
			}
			if _, err := extractCreatedObjectNames("SELECT " + sql); err == nil {
				t.Fatal("collection accepted invalid Unicode")
			}
			if _, err := qualifyFunctionBodiesWithTempSchema("DO "+sql, "public", "tmp", schemaRemapNames{}); err == nil {
				t.Fatal("remapping accepted invalid Unicode")
			}
		})
	}
}

func TestUnicodeSchemaAndObjectRemapping(t *testing.T) {
	const original = `-- public.t
CREATE TABLE u&"!0070ublic" /* keep */ UESCAPE /* also */ '!' . U&"t\+01F600"(id int);
CREATE UNIQUE INDEX U&"i\D83D\DE00" ON U&"public".U&"t\+01F600"(id);
CREATE TABLE other.U&"public"(id int);
SELECT 'public.t', U&'\0070ublic.t';`
	stripped, err := stripSchemaQualifications(original, "public")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, `u&"!0070ublic" /* keep */ UESCAPE /* also */ '!' .`, ` /* keep */  /* also */  `, 1)
	want = strings.Replace(want, `U&"public".`, "", 1)
	if stripped != want {
		t.Fatalf("got %s; want %s", stripped, want)
	}
	objects, err := extractCreatedObjectNames(stripped)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(objects, schemaRemapNames{body: map[string]bool{"t😀": true}, relations: map[string]bool{"t😀": true, "i😀": true}}) {
		t.Fatalf("objects %+v", objects)
	}
	const body = `DO u&'BEGIN PERFORM U&''!0070ublic."t!+01F600"'' /* keep */ UESCAPE ''!''::regclass; PERFORM ''public.t'', public.t; END';`
	got, err := qualifyFunctionBodiesWithTempSchema(body, "public", "tmp", objects)
	if err != nil {
		t.Fatal(err)
	}
	const expected = `DO u&'BEGIN PERFORM U&''"tmp"."t😀"'' /* keep */  ::regclass; PERFORM ''public.t'', public.t; END';`
	if got != expected {
		t.Fatalf("got %s; want %s", got, expected)
	}
	const unchanged = `DO U&'BEGIN RAISE NOTICE ''\0070ublic.t''; END' UESCAPE '\';`
	got, err = qualifyFunctionBodiesWithTempSchema(unchanged, "public", "tmp", objects)
	if err != nil || got != unchanged {
		t.Fatalf("changed unrelated Unicode text %s, %v", got, err)
	}
}

func TestQuotedKeywordsAreIdentifiers(t *testing.T) {
	const sql = `SELECT "do", U&'public.t'; CREATE TABLE U&"on"(id int); CREATE INDEX U&"only" ON U&"on"(id);`
	got, err := qualifyFunctionBodiesWithTempSchema(sql, "public", "tmp", schemaRemapNames{})
	if err != nil || got != sql {
		t.Fatalf("quoted keyword interpreted as code %s, %v", got, err)
	}
	objects, err := extractCreatedObjectNames(sql)
	if err != nil || !objects.relations["only"] || !objects.relations["on"] {
		t.Fatalf("quoted names lost %+v, %v", objects, err)
	}
}

func TestUnicodeRegclassDoesNotCloseDollarBody(t *testing.T) {
	const sql = `DO $body$ BEGIN PERFORM U&'public."t\0024body\0024\0024body_\0024"'::regclass; END $body$;`
	got, err := qualifyFunctionBodiesWithTempSchema(sql, "public", "tmp", schemaRemapNames{})
	const want = `DO $body__$ BEGIN PERFORM U&'"tmp"."t$body$$body_$"'::regclass; END $body__$;`
	if err != nil || got != want {
		t.Fatalf("got %s, %v; want %s", got, err, want)
	}
}
