CREATE TYPE action_type AS ENUM (
    'pending',
    'approved',
    'rejected'
);

ALTER TABLE user_pending_permissions ALTER COLUMN id TYPE bigint USING id::bigint;

ALTER TABLE user_pending_permissions ALTER COLUMN user_id TYPE bigint USING user_id::bigint;

ALTER TABLE user_pending_permissions ALTER COLUMN object_ids_ints TYPE bigint[] USING object_ids_ints::bigint[];

ALTER TABLE user_pending_permissions ALTER COLUMN action TYPE action_type USING action::action_type;

ALTER TABLE user_pending_permissions ALTER COLUMN status DROP DEFAULT;

ALTER TABLE user_pending_permissions ALTER COLUMN status TYPE action_type USING status::action_type;

ALTER TABLE user_pending_permissions ALTER COLUMN status SET DEFAULT 'pending'::action_type;

ALTER TABLE user_pending_permissions ALTER COLUMN tags TYPE action_type[] USING tags::action_type[];

ALTER TABLE user_pending_permissions ALTER COLUMN amount TYPE numeric(20,6);

ALTER TABLE user_pending_permissions ALTER COLUMN arfcn_dl DROP DEFAULT;

ALTER TABLE user_pending_permissions ALTER COLUMN arfcn_dl TYPE integer USING arfcn_dl::integer;

ALTER TABLE user_pending_permissions ALTER COLUMN arfcn_dl SET DEFAULT 0;

ALTER TABLE user_pending_permissions ALTER COLUMN priority TYPE integer USING priority::integer;

ALTER TABLE user_pending_permissions ALTER COLUMN flag DROP DEFAULT;

ALTER TABLE user_pending_permissions ALTER COLUMN flag TYPE mystatus USING flag::mystatus;

ALTER TABLE user_pending_permissions ALTER COLUMN flag SET DEFAULT 'on'::mystatus;
