-- Create "context_summary_revisions" table
CREATE TABLE "context_summary_revisions" (
  "room_id" uuid NOT NULL,
  "revision" bigint NOT NULL DEFAULT 0,
  PRIMARY KEY ("room_id"),
  CONSTRAINT "context_summary_revisions_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
