ALTER TABLE payout DROP CONSTRAINT payout_id_check;
ALTER TABLE payout
ADD CONSTRAINT payout_id_check CHECK (id IS NULL OR id::text LIKE 'p-%' AND partner_id IS NOT NULL);
