-- Create "subscriptions" table
CREATE TABLE "subscriptions" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "stripe_customer_id" character varying(255) NOT NULL,
  "stripe_subscription_id" character varying(255) NOT NULL,
  "stripe_price_id" character varying(255) NOT NULL,
  "plan_code" character varying(50) NOT NULL,
  "status" character varying(50) NOT NULL,
  "monthly_token_allocation" bigint NOT NULL,
  "current_period_start" timestamptz NOT NULL,
  "current_period_end" timestamptz NOT NULL,
  "cancel_at_period_end" boolean NOT NULL DEFAULT false,
  "canceled_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "subscriptions_stripe_subscription_id_unique" UNIQUE ("stripe_subscription_id"),
  CONSTRAINT "subscriptions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_subscriptions_user_id" to table: "subscriptions"
CREATE INDEX "idx_subscriptions_user_id" ON "subscriptions" ("user_id");
-- Create "payment_history" table
CREATE TABLE "payment_history" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "subscription_id" uuid NULL,
  "payment_rail" character varying(20) NOT NULL DEFAULT 'stripe',
  "stripe_event_id" character varying(255) NOT NULL,
  "stripe_reference_id" character varying(255) NOT NULL DEFAULT '',
  "kind" character varying(20) NOT NULL,
  "amount_cents" bigint NOT NULL,
  "currency" character varying(10) NOT NULL DEFAULT 'usd',
  "tokens_credited" bigint NOT NULL DEFAULT 0,
  "status" character varying(20) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "payment_history_stripe_event_id_unique" UNIQUE ("stripe_event_id"),
  CONSTRAINT "payment_history_subscription_id_fkey" FOREIGN KEY ("subscription_id") REFERENCES "subscriptions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL,
  CONSTRAINT "payment_history_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_payment_history_user_id" to table: "payment_history"
CREATE INDEX "idx_payment_history_user_id" ON "payment_history" ("user_id", "created_at" DESC);
