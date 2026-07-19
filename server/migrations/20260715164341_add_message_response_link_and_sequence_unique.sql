-- Modify "messages" table
ALTER TABLE "messages" ADD COLUMN "in_response_to_message_id" uuid NULL, ADD CONSTRAINT "messages_room_sequence_unique" UNIQUE ("room_id", "sequence"), ADD CONSTRAINT "messages_in_response_to_message_id_fkey" FOREIGN KEY ("in_response_to_message_id") REFERENCES "messages" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
