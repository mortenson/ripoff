CREATE TABLE uuid_users (
  id UUID NOT NULL PRIMARY KEY,
  email TEXT NOT NULL
);

CREATE TABLE int_users (
  id BIGSERIAL NOT NULL,
  email TEXT NOT NULL,
  PRIMARY KEY (id)
);
