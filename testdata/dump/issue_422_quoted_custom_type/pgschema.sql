--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.5
-- Dumped by pgschema version 1.10.0


--
-- Name: MyComposite; Type: TYPE; Schema: -; Owner: -
--

CREATE TYPE "MyComposite" AS (a integer, b text);

--
-- Name: MyDomain; Type: DOMAIN; Schema: -; Owner: -
--

CREATE DOMAIN "MyDomain" AS integer
  CONSTRAINT "MyDomain_check" CHECK (VALUE > 0);

--
-- Name: MyStatus; Type: TYPE; Schema: -; Owner: -
--

CREATE TYPE "MyStatus" AS ENUM (
    'active',
    'inactive'
);

--
-- Name: items; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS items (
    id bigint,
    status "MyStatus" DEFAULT 'active'::"MyStatus" NOT NULL,
    tags "MyStatus"[],
    payload "MyComposite",
    quantity "MyDomain",
    CONSTRAINT items_pkey PRIMARY KEY (id)
);

