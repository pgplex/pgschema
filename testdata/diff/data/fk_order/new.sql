CREATE TABLE plan_tier (
    id text PRIMARY KEY,
    name text NOT NULL,
    seat_limit integer
);

CREATE TABLE feature_flag (
    key text NOT NULL,
    tier text NOT NULL REFERENCES plan_tier(id),
    description text,
    PRIMARY KEY (key, tier)
);

\copy plan_tier (id, name, seat_limit) FROM 'data/plan_tier.csv' WITH (FORMAT csv, HEADER)
\copy feature_flag (key, tier, description) FROM 'data/feature_flag.csv' WITH (FORMAT csv, HEADER)
