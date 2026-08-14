CREATE TABLE base ();

CREATE DOMAIN d AS base;

CREATE TABLE uses (
    dcol d
);
