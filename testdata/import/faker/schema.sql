CREATE TABLE users (
  id UUID NOT NULL,
  email TEXT NOT NULL,
  last_logged_in_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (id)
);
