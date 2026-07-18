-- Create "message_context_summaries" table
CREATE TABLE "message_context_summaries" (
  "room_id" uuid NOT NULL,
  "model" character varying(100) NOT NULL,
  "covered_up_to_sequence" bigint NOT NULL,
  "summary_text" text NOT NULL,
  "token_count" integer NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("room_id"),
  CONSTRAINT "message_context_summaries_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
