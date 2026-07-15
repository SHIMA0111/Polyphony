-- Modify "rooms" table
ALTER TABLE "rooms" ADD COLUMN "ai_provider" character varying(50) NULL, ADD COLUMN "ai_model" character varying(100) NULL;
