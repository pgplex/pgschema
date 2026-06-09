--
-- Test case for GitHub issue #422: Quoted mixed-case custom types cannot be round-tripped
--
-- A column whose type is a mixed-case user-defined type must reference that type
-- with double quotes (status "MyStatus"). Without quoting, the dumped SQL folds the
-- type name to lower case (mystatus), which does not exist, breaking round-trip.
--

CREATE TYPE "MyStatus" AS ENUM ('active', 'inactive');

CREATE TYPE "MyComposite" AS (
    a integer,
    b text
);

CREATE DOMAIN "MyDomain" AS integer CHECK (VALUE > 0);

CREATE TABLE items (
    id        bigint NOT NULL,
    status    "MyStatus" DEFAULT 'active'::"MyStatus" NOT NULL,
    tags      "MyStatus"[],
    payload   "MyComposite",
    quantity  "MyDomain",
    CONSTRAINT items_pkey PRIMARY KEY (id)
);
