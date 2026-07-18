-- Modify "messages" table
ALTER TABLE "messages" ADD COLUMN "is_deleted" boolean NOT NULL DEFAULT false, ADD COLUMN "exclude_from_ai" boolean NOT NULL DEFAULT false;
-- Modify "rooms" table
ALTER TABLE "rooms" ADD COLUMN "ai_context_cutoff_at" timestamptz NULL;
