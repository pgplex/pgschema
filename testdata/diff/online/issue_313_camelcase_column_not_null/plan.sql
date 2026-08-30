ALTER TABLE "Planning" ADD CONSTRAINT "Planning_offersValidUntil_not_null" NOT NULL "offersValidUntil" NOT VALID;

ALTER TABLE "Planning" VALIDATE CONSTRAINT "Planning_offersValidUntil_not_null";
