-- Modify "subscriptions" table
ALTER TABLE "subscriptions" ADD COLUMN "stripe_checkout_session_id" character varying(255) NOT NULL DEFAULT '';
