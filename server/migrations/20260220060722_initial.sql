-- Create "users" table
CREATE TABLE "users" (
  "id" uuid NOT NULL,
  "email" character varying(255) NOT NULL,
  "username" character varying(100) NOT NULL,
  "password_hash" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "users_email_unique" UNIQUE ("email"),
  CONSTRAINT "users_username_unique" UNIQUE ("username")
);
-- Create "rooms" table
CREATE TABLE "rooms" (
  "id" uuid NOT NULL,
  "name" character varying(255) NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "owner_id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "rooms_owner_id_fkey" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT
);
-- Create "messages" table
CREATE TABLE "messages" (
  "id" uuid NOT NULL,
  "room_id" uuid NOT NULL,
  "sender_id" uuid NULL,
  "content" text NOT NULL,
  "type" character varying(20) NOT NULL,
  "status" character varying(20) NOT NULL DEFAULT 'completed',
  "sequence" bigint NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "messages_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "messages_sender_id_fkey" FOREIGN KEY ("sender_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL
);
-- Create index "idx_messages_room_sequence" to table: "messages"
CREATE INDEX "idx_messages_room_sequence" ON "messages" ("room_id", "sequence" DESC);
-- Create "room_members" table
CREATE TABLE "room_members" (
  "id" uuid NOT NULL,
  "room_id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "role" character varying(50) NOT NULL DEFAULT 'member',
  "joined_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "room_members_room_id_user_id_key" UNIQUE ("room_id", "user_id"),
  CONSTRAINT "room_members_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "room_members_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT
);
-- Create "room_sequences" table
CREATE TABLE "room_sequences" (
  "room_id" uuid NOT NULL,
  "next_sequence" bigint NOT NULL DEFAULT 1,
  PRIMARY KEY ("room_id"),
  CONSTRAINT "room_sequences_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
