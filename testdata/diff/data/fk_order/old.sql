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

INSERT INTO plan_tier (id, name, seat_limit) VALUES ('free', 'Free', 3), ('legacy', 'Legacy', 5), ('team', 'Team', 25);
INSERT INTO feature_flag (key, tier, description) VALUES ('audit_log', 'team', 'Audit log retention'), ('old_flag', 'legacy', 'Retired flag');
