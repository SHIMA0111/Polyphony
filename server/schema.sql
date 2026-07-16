-- Desired database schema (Atlas declarative).
-- Run `atlas schema apply` to converge the database to this state.

CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    username VARCHAR(100) NOT NULL,
    password_hash TEXT NOT NULL,
    kratos_identity_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_username_unique UNIQUE (username),
    CONSTRAINT users_kratos_identity_id_unique UNIQUE (kratos_identity_id)
);

CREATE TABLE rooms (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    ai_context_cutoff_at TIMESTAMPTZ,
    ai_provider VARCHAR(50) NULL,
    ai_model VARCHAR(100) NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE room_members (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(room_id, user_id),
    CONSTRAINT room_members_role_check CHECK (role IN ('reader', 'guest', 'member', 'admin', 'master'))
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
    in_response_to_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    exclude_from_ai BOOLEAN NOT NULL DEFAULT false,
    visibility VARCHAR(20) NOT NULL DEFAULT 'public',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT messages_room_sequence_unique UNIQUE (room_id, sequence),
    CONSTRAINT messages_visibility_check CHECK (visibility IN ('public', 'private'))
);

CREATE INDEX idx_messages_room_sequence ON messages(room_id, sequence DESC);

CREATE TABLE message_attachments (
    id UUID PRIMARY KEY,
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    s3_key VARCHAR(512) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_message_attachments_message_id ON message_attachments(message_id);

CREATE TABLE room_invitations (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    inviter_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    invitee_id UUID REFERENCES users(id) ON DELETE CASCADE,
    invite_code VARCHAR(64) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT room_invitations_invite_code_unique UNIQUE (invite_code)
);

CREATE INDEX idx_room_invitations_room_id ON room_invitations(room_id);
CREATE INDEX idx_room_invitations_invitee_id ON room_invitations(invitee_id) WHERE invitee_id IS NOT NULL;

CREATE TABLE token_balances (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    balance BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE token_transactions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    room_id UUID REFERENCES rooms(id) ON DELETE SET NULL,
    message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    type VARCHAR(20) NOT NULL,
    amount BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT token_transactions_type_check CHECK (type IN ('consumption', 'charge', 'adjustment'))
);

CREATE INDEX idx_token_transactions_user_created ON token_transactions(user_id, created_at DESC, id DESC);

CREATE TABLE groups (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE group_members (
    id UUID PRIMARY KEY,
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(group_id, user_id)
);

CREATE INDEX idx_groups_owner_id ON groups(owner_id);
CREATE INDEX idx_group_members_group_id ON group_members(group_id);
