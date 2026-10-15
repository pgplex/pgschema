CREATE TABLE public.code (
    code integer PRIMARY KEY CONSTRAINT code_check CHECK (code > 0 AND code < 255),
    a varchar(20),
    b varchar(20),
    CONSTRAINT code_a_check CHECK (a IS NULL OR (a LIKE 'p-%' AND b IS NOT NULL))
);
