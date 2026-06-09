--
-- Schema for GitHub issue #422: quoted mixed-case custom type names
--

CREATE TYPE "MyStatus" AS ENUM ('active', 'inactive');

CREATE TYPE "MyComposite" AS (
    a integer,
    b text
);

CREATE DOMAIN "MyDomain" AS integer CONSTRAINT "MyDomain_check" CHECK (VALUE > 0);

CREATE TABLE items (
    id        bigint NOT NULL,
    status    "MyStatus" DEFAULT 'active'::"MyStatus" NOT NULL,
    tags      "MyStatus"[],
    payload   "MyComposite",
    quantity  "MyDomain",
    CONSTRAINT items_pkey PRIMARY KEY (id)
);
