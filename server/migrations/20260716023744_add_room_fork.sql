-- Modify "rooms" table
ALTER TABLE "rooms" ADD COLUMN "forked_from_room_id" uuid NULL, ADD COLUMN "is_archived" boolean NOT NULL DEFAULT false, ADD CONSTRAINT "rooms_forked_from_room_id_fkey" FOREIGN KEY ("forked_from_room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- Create "room_fork_jobs" table
CREATE TABLE "room_fork_jobs" (
  "id" uuid NOT NULL,
  "source_room_id" uuid NOT NULL,
  "new_room_id" uuid NOT NULL,
  "status" character varying(20) NOT NULL DEFAULT 'pending',
  "total_messages" bigint NOT NULL DEFAULT 0,
  "copied_messages" bigint NOT NULL DEFAULT 0,
  "error_message" text NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "room_fork_jobs_new_room_id_fkey" FOREIGN KEY ("new_room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "room_fork_jobs_source_room_id_fkey" FOREIGN KEY ("source_room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_room_fork_jobs_new_room" to table: "room_fork_jobs"
CREATE INDEX "idx_room_fork_jobs_new_room" ON "room_fork_jobs" ("new_room_id");
