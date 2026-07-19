-- Create "token_balances" table
CREATE TABLE "token_balances" (
  "user_id" uuid NOT NULL,
  "balance" bigint NOT NULL DEFAULT 0,
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("user_id"),
  CONSTRAINT "token_balances_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create "token_transactions" table
CREATE TABLE "token_transactions" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "room_id" uuid NULL,
  "message_id" uuid NULL,
  "type" character varying(20) NOT NULL,
  "amount" bigint NOT NULL,
  "balance_after" bigint NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "token_transactions_message_id_fkey" FOREIGN KEY ("message_id") REFERENCES "messages" ("id") ON UPDATE NO ACTION ON DELETE SET NULL,
  CONSTRAINT "token_transactions_room_id_fkey" FOREIGN KEY ("room_id") REFERENCES "rooms" ("id") ON UPDATE NO ACTION ON DELETE SET NULL,
  CONSTRAINT "token_transactions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "token_transactions_type_check" CHECK ((type)::text = ANY ((ARRAY['consumption'::character varying, 'charge'::character varying, 'adjustment'::character varying])::text[]))
);
-- Create index "idx_token_transactions_user_created" to table: "token_transactions"
CREATE INDEX "idx_token_transactions_user_created" ON "token_transactions" ("user_id", "created_at" DESC, "id" DESC);
