-- Create "message_attachments" table
CREATE TABLE "message_attachments" (
  "id" uuid NOT NULL,
  "room_id" uuid NOT NULL,
  "message_id" uuid NULL,
  "s3_key" character varying(512) NOT NULL,
  "mime_type" character varying(100) NOT NULL,
  "size_bytes" bigint NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "message_attachments_message_id_fkey" FOREIGN KEY ("message_id") REFERENCES "messages" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "message_attachments_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_message_attachments_message_id" to table: "message_attachments"
CREATE INDEX "idx_message_attachments_message_id" ON "message_attachments" ("message_id");
-- Create index "idx_message_attachments_room_id" to table: "message_attachments"
CREATE INDEX "idx_message_attachments_room_id" ON "message_attachments" ("room_id");
