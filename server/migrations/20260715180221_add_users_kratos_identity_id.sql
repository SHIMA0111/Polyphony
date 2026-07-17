-- Modify "users" table
ALTER TABLE "users" ADD COLUMN "kratos_identity_id" uuid NULL, ADD CONSTRAINT "users_kratos_identity_id_unique" UNIQUE ("kratos_identity_id");
