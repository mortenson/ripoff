CREATE TYPE user_role AS ENUM ('admin', 'power', 'normie', 'banned');

CREATE TABLE users (
  id UUID NOT NULL PRIMARY KEY,
  email TEXT NOT NULL,
  role user_role NOT NULL
);

CREATE TABLE workspaces (
  id          UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
  slug        TEXT           NOT NULL
);

CREATE TABLE workspace_memberships (
  user_id         UUID     NOT NULL,
  workspace_id    UUID     NOT NULL,
  PRIMARY KEY (user_id, workspace_id)
);
