-- An explicitly-declared standalone sequence referenced by an integer column via a
-- nextval() default, WITHOUT a real OWNED BY edge. This is NOT a SERIAL column.
-- pgschema must keep the sequence standalone (bigint default MAXVALUE, no ownership)
-- and render the column as "integer ... DEFAULT nextval(...)" rather than collapsing it
-- to serial (which would create an integer-capped, column-owned sequence instead).
CREATE SEQUENCE "dssAnswerSplit_dssSplitAnswerId_seq";

CREATE TABLE "dssAnswerSplit" (
    "dssSplitAnswerId" integer NOT NULL DEFAULT nextval('"dssAnswerSplit_dssSplitAnswerId_seq"'::regclass),
    CONSTRAINT "dssAnswerSplit_pkey" PRIMARY KEY ("dssSplitAnswerId")
);
