ALTER TABLE metric_refs DROP CONSTRAINT metric_refs_tripled_fkey;

ALTER TABLE metrics DROP COLUMN tripled;

ALTER TABLE metrics
ADD COLUMN tripled integer GENERATED ALWAYS AS ((a * 3)) STORED CONSTRAINT metrics_tripled_key UNIQUE;

ALTER TABLE metrics ALTER COLUMN doubled SET EXPRESSION AS ((a * 2));

ALTER TABLE metrics ALTER COLUMN label DROP EXPRESSION;

ALTER TABLE metrics ALTER COLUMN label SET DEFAULT 'none';

ALTER TABLE metrics
ADD CONSTRAINT metrics_tripled_check CHECK (tripled > 0);

DROP INDEX IF EXISTS metrics_tripled_idx;

CREATE INDEX IF NOT EXISTS metrics_tripled_idx ON metrics ((tripled + 1)) WHERE (tripled > 10);

ALTER TABLE metric_refs
ADD CONSTRAINT metric_refs_tripled_fkey FOREIGN KEY (tripled) REFERENCES metrics (tripled);
