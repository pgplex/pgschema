package postgres

import (
	"strings"
	"testing"
)

func TestRemapExecutableDOBodies(t *testing.T) {
	for _, tc := range []struct {
		name, sql, schema, want string
		objects                 map[string]bool
	}{
		{name: "code_comments_and_literals", schema: "public", sql: `DO $b$ BEGIN
PERFORM 1 FROM public.t;
-- public.t $$
/* public.t /* nested public.t */ public.t */
PERFORM 'public.t', E'it\'s public.t', $text$public.t$text$;
EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', 'public', 'old', 'public.t');
PERFORM 1 FROM other.t;
END $b$;`, want: `DO $b$ BEGIN
PERFORM 1 FROM public.t;
-- public.t $$
/* public.t /* nested public.t */ public.t */
PERFORM 'public.t', E'it\'s public.t', $text$public.t$text$;
EXECUTE pg_catalog.format('ALTER INDEX %I.%I RENAME TO %I', 'public', 'old', 'public.t');
PERFORM 1 FROM other.t;
END $b$;`},
		{name: "regclass_literals", schema: "public", sql: `DO LANGUAGE plpgsql $$ BEGIN
PERFORM 'public.t'::regclass, E'public."t''\\"'::pg_catalog.regclass, $n$public.t$n$::regclass;
PERFORM CAST('public.t' AS pg_catalog.regclass), 'other.t'::regclass, 'public.t'::text, 'public.t'::other.regclass;
END $$;`, want: `DO LANGUAGE plpgsql $$ BEGIN
PERFORM '"pgschema_tmp_x".t'::regclass, E'"pgschema_tmp_x"."t''\\"'::pg_catalog.regclass, $n$"pgschema_tmp_x".t$n$::regclass;
PERFORM CAST('"pgschema_tmp_x".t' AS pg_catalog.regclass), 'other.t'::regclass, 'public.t'::text, 'public.t'::other.regclass;
END $$;`},
		{name: "external_objects", schema: "public", objects: map[string]bool{"t": true}, sql: `DO /* public.t */ $$ BEGIN PERFORM public.unaccent('public.t') FROM public.t; PERFORM 'public.t'::regclass, 'public.extension_table'::regclass; END $$;`, want: `DO /* public.t */ $$ BEGIN PERFORM public.unaccent('public.t') FROM public.t; PERFORM '"pgschema_tmp_x".t'::regclass, 'public.extension_table'::regclass; END $$;`},
		{name: "single_quoted_do", schema: "public", sql: `DO 'BEGIN PERFORM 1 FROM public.t; PERFORM ''public.t''::regclass; END';`, want: `DO 'BEGIN PERFORM 1 FROM public.t; PERFORM ''"pgschema_tmp_x".t''::regclass; END';`},
		{name: "escaped_identifiers", schema: `s".'\$$`, sql: `DO $body$ BEGIN PERFORM 1 FROM "s"".'\$$"."t.'\$$"; PERFORM '"s"".''\$$"."t.''\$$"'::pg_catalog.regclass; END $body$;`, want: `DO $body$ BEGIN PERFORM 1 FROM "s"".'\$$"."t.'\$$"; PERFORM '"pgschema_tmp_x"."t.''\$$"'::pg_catalog.regclass; END $body$;`},
		{name: "preserve_non_executable_text", schema: "public", sql: `-- DO $$ public.t $$
SELECT 'DO', 'public.t'; COMMENT ON TABLE t IS $$ public.t $$;`, want: `-- DO $$ public.t $$
SELECT 'DO', 'public.t'; COMMENT ON TABLE t IS $$ public.t $$;`},
		{name: "repeated_schema_identifiers", schema: "public", sql: `DO $$ BEGIN PERFORM public.public.id FROM public.public; PERFORM other.public.id FROM other.public; END $$;`, want: `DO $$ BEGIN PERFORM public.public.id FROM public.public; PERFORM other.public.id FROM other.public; END $$;`},
		{name: "unterminated_dollar_body", schema: "public", sql: `DO $$ public.t`, want: `DO $$ public.t`},
		{name: "unterminated_string_body", schema: "public", sql: `DO 'public.t`, want: `DO 'public.t`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := qualifyFunctionBodiesWithTempSchema(tc.sql, tc.schema, "pgschema_tmp_x", schemaRemapNames{body: tc.objects, relations: tc.objects})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestCreatedObjectsWithLiteralDelimiters(t *testing.T) {
	const schema = `s".'\$$`
	sql := `-- CREATE TABLE bogus(id int);
CREATE TABLE "s"".'\$$"."t"".'\$$"(id int);
COMMENT ON TABLE "s"".'\$$"."t"".'\$$" IS 'CREATE TABLE bogus2(id int)';
CREATE TABLE other.external(id int);
DO $$ BEGIN PERFORM 1; END $$;`
	stripped, err := stripSchemaQualifications(sql, schema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stripped, `CREATE TABLE "t"".'\$$"`) {
		t.Fatalf("schema not stripped: %s", stripped)
	}
	objects, err := extractCreatedObjectNames(stripped)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects.body) != 1 || !objects.body[`t".'\$$`] || len(objects.relations) != 1 || !objects.relations[`t".'\$$`] {
		t.Fatalf("incorrect created objects: %v", objects)
	}
}

func TestStripRepeatedSchemaIdentifier(t *testing.T) {
	const sql = `COMMENT ON COLUMN public.public.id IS 'public.public.id'; SELECT other.public.id FROM other.public;`
	const want = `COMMENT ON COLUMN public.id IS 'public.public.id'; SELECT other.public.id FROM other.public;`
	got, err := stripSchemaQualifications(sql, "public")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestDORegclassRemappingPreservesAliasesAndRecords(t *testing.T) {
	for _, body := range []string{
		`BEGIN PERFORM public.t FROM t AS public; END`,
		`BEGIN PERFORM "public".t FROM t AS "public"; END`,
		`DECLARE public record; BEGIN SELECT 1 AS t INTO public; PERFORM public.t; END`,
		`DECLARE public record; BEGIN SELECT 1 AS t INTO public; DECLARE public record; BEGIN SELECT 2 AS t INTO public; PERFORM public.t; END; PERFORM public.t; END`,
	} {
		t.Run(body, func(t *testing.T) {
			for _, objects := range []map[string]bool{nil, {"t": true}} {
				// The second block forces a remap in the same script. Alias and
				// record references in the first block must still remain verbatim.
				original := "DO $$ " + body + " $$; DO $$ BEGIN PERFORM 'public.t'::regclass; END $$;"
				want := "DO $$ " + body + " $$; DO $$ BEGIN PERFORM '\"pgschema_tmp_x\".t'::regclass; END $$;"
				got, err := qualifyFunctionBodiesWithTempSchema(original, "public", "pgschema_tmp_x", schemaRemapNames{body: objects, relations: objects})
				if err != nil || got != want {
					t.Fatalf("remapping changed an alias or record reference:\ngot: %s\nwant: %s\nerror: %v", got, want, err)
				}
			}
		})
	}
}
