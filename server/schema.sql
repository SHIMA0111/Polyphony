-- Desired database schema (Atlas declarative).
-- Run `atlas schema apply` to converge the database to this state.

CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    username VARCHAR(100) NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_username_unique UNIQUE (username)
);

CREATE TABLE rooms (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE room_members (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(room_id, user_id)
);

CREATE TABLE room_sequences (
    room_id UUID PRIMARY KEY REFERENCES rooms(id) ON DELETE CASCADE,
    next_sequence BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE messages (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'completed',
    sequence BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_room_sequence ON messages(room_id, sequence DESC);
