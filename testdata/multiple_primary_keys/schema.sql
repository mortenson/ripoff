CREATE TABLE users (
  id UUID NOT NULL PRIMARY KEY,
  email TEXT NOT NULL
);

CREATE TABLE workspaces (
  id UUID NOT NULL PRIMARY KEY,
  name TEXT NOT NULL
);

CREATE TABLE workspace_users (
  user_id UUID NOT NULL REFERENCES users,
  workspace_id UUID NOT NULL REFERENCES workspaces,
  PRIMARY KEY (user_id, workspace_id)
);

CREATE TABLE workspace_user_permissions (
  user_id UUID NOT NULL REFERENCES users,
  workspace_id UUID NOT NULL REFERENCES workspaces,
  is_admin BOOLEAN NOT NULL,
  PRIMARY KEY (user_id, workspace_id)
);
