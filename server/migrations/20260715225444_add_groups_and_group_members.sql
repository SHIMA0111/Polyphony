-- Create "groups" table
CREATE TABLE "groups" (
  "id" uuid NOT NULL,
  "owner_id" uuid NOT NULL,
  "name" character varying(255) NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "groups_owner_id_fkey" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_groups_owner_id" to table: "groups"
CREATE INDEX "idx_groups_owner_id" ON "groups" ("owner_id");
-- Create "group_members" table
CREATE TABLE "group_members" (
  "id" uuid NOT NULL,
  "group_id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "added_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "group_members_group_id_user_id_key" UNIQUE ("group_id", "user_id"),
  CONSTRAINT "group_members_group_id_fkey" FOREIGN KEY ("group_id") REFERENCES "groups" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "group_members_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_group_members_user_id" to table: "group_members"
CREATE INDEX "idx_group_members_user_id" ON "group_members" ("user_id");
