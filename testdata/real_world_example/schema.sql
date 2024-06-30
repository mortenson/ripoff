CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE workspaces (
  id          UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
  slug        CITEXT         NOT NULL UNIQUE,
  name        TEXT           NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
  id          UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
  email       CITEXT         NOT NULL UNIQUE,
  avatar_url  TEXT           NULL,
  name        TEXT           NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE groups (
  id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  name            CITEXT        NOT NULL,
  description     TEXT          NOT NULL,
  workspace_id    UUID          NOT NULL,
  created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE workspace_memberships (
  id              UUID     PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID     NOT NULL,
  workspace_id    UUID     NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX workspace_memberships_user_workspace_index ON workspace_memberships (user_id, workspace_id);

CREATE TABLE group_memberships (
  id              UUID     PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID     NOT NULL,
  group_id        UUID     NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX group_memberships_user_group_index ON group_memberships (user_id, group_id);
