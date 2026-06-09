CREATE TYPE action_type AS ENUM (
    'pending',
    'approved',
    'rejected'
);

ALTER TABLE user_pending_permissions ALTER COLUMN id TYPE bigint;

ALTER TABLE user_pending_permissions ALTER COLUMN user_id TYPE bigint;

ALTER TABLE user_pending_permissions ALTER COLUMN object_ids_ints TYPE bigint[];

ALTER TABLE user_pending_permissions ALTER COLUMN action TYPE action_type USING action::action_type;

ALTER TABLE user_pending_permissions ALTER COLUMN status DROP DEFAULT;

ALTER TABLE user_pending_permissions ALTER COLUMN status TYPE action_type USING status::action_type;

ALTER TABLE user_pending_permissions ALTER COLUMN status SET DEFAULT 'pending'::action_type;

ALTER TABLE user_pending_permissions ALTER COLUMN tags TYPE action_type[] USING tags::action_type[];

ALTER TABLE user_pending_permissions ALTER COLUMN amount TYPE numeric(20,6);
