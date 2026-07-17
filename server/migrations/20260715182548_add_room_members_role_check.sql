-- Modify "room_members" table
ALTER TABLE "room_members" ADD CONSTRAINT "room_members_role_check" CHECK ((role)::text = ANY ((ARRAY['reader'::character varying, 'guest'::character varying, 'member'::character varying, 'admin'::character varying, 'master'::character varying])::text[]));
