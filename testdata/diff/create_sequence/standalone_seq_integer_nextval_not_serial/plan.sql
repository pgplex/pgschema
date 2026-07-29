CREATE SEQUENCE IF NOT EXISTS "dssAnswerSplit_dssSplitAnswerId_seq";

CREATE TABLE IF NOT EXISTS "dssAnswerSplit" (
    "dssSplitAnswerId" integer DEFAULT nextval('"dssAnswerSplit_dssSplitAnswerId_seq"'::regclass),
    CONSTRAINT "dssAnswerSplit_pkey" PRIMARY KEY ("dssSplitAnswerId")
);
