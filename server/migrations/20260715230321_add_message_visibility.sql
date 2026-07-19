-- Modify "messages" table
ALTER TABLE "messages" ADD CONSTRAINT "messages_visibility_check" CHECK ((visibility)::text = ANY ((ARRAY['public'::character varying, 'private'::character varying])::text[])), ADD COLUMN "visibility" character varying(20) NOT NULL DEFAULT 'public';
