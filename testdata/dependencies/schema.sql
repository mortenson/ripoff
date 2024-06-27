CREATE TABLE avatars (
  id UUID NOT NULL PRIMARY KEY,
  url TEXT NOT NULL
);

CREATE TABLE avatar_modifiers (
  /* Both an primary and foreign key, yikes! */
  id UUID NOT NULL PRIMARY KEY REFERENCES avatars,
  grayscale BOOLEAN NOT NULL
);

CREATE TABLE users (
  id UUID NOT NULL PRIMARY KEY,
  avatar_id UUID NOT NULL REFERENCES avatars,
  email TEXT NOT NULL
);
