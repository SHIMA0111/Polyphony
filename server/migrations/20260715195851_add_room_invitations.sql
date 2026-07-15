-- Create "room_invitations" table
CREATE TABLE "room_invitations" (
  "id" uuid NOT NULL,
  "room_id" uuid NOT NULL,
  "inviter_id" uuid NOT NULL,
  "invitee_id" uuid NULL,
  "invite_code" character varying(64) NOT NULL,
  "role" character varying(50) NOT NULL DEFAULT 'member',
  "status" character varying(20) NOT NULL DEFAULT 'pending',
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "room_invitations_invite_code_unique" UNIQUE ("invite_code"),
  CONSTRAINT "room_invitations_invitee_id_fkey" FOREIGN KEY ("invitee_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "room_invitations_inviter_id_fkey" FOREIGN KEY ("inviter_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT,
  CONSTRAINT "room_invitations_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_room_invitations_invitee_id" to table: "room_invitations"
CREATE INDEX "idx_room_invitations_invitee_id" ON "room_invitations" ("invitee_id") WHERE (invitee_id IS NOT NULL);
-- Create index "idx_room_invitations_room_id" to table: "room_invitations"
CREATE INDEX "idx_room_invitations_room_id" ON "room_invitations" ("room_id");
