INSERT INTO plan_tier (id, name, seat_limit) VALUES ('enterprise', 'Enterprise', NULL);

INSERT INTO feature_flag (key, tier, description) VALUES ('sso', 'enterprise', 'Single sign-on');

UPDATE plan_tier SET seat_limit = 50 WHERE id = 'team';

DELETE FROM feature_flag WHERE key = 'old_flag' AND tier = 'legacy';

DELETE FROM plan_tier WHERE id = 'legacy';
