CREATE TABLE avatars (
  id UUID NOT NULL PRIMARY KEY,
  url TEXT NOT NULL
);

CREATE TABLE avatar_modifiers (
  id UUID NOT NULL PRIMARY KEY REFERENCES avatars,
  grayscale BOOLEAN NOT NULL
);

CREATE TABLE roles (
  id UUID NOT NULL PRIMARY KEY,
  name TEXT NOT NULL
);

ALTER TABLE roles ADD CONSTRAINT unique_roles_name UNIQUE (name);

CREATE TABLE employees (
  id BIGSERIAL NOT NULL PRIMARY KEY,
  role TEXT NOT NULL
);

-- We are foreign keying to a non primary key. Tricky!
ALTER TABLE employees
  ADD CONSTRAINT fk_employees_roles
  FOREIGN KEY (role) REFERENCES roles (name);

CREATE TABLE users (
  id UUID NOT NULL PRIMARY KEY,
  avatar_id UUID NOT NULL REFERENCES avatars,
  email TEXT NOT NULL,
  employee_id BIGSERIAL NOT NULL REFERENCES employees
);

CREATE TABLE multi_column_pkey (
  id1 UUID NOT NULL,
  id2 UUID NOT NULL,
  PRIMARY KEY (id1, id2)
);

CREATE TABLE multi_column_fkey (
  id UUID NOT NULL PRIMARY KEY,
  id1_fkey UUID NOT NULL,
  id2_fkey UUID NOT NULL
);

ALTER TABLE multi_column_fkey
  ADD CONSTRAINT multi_column_fkey_multi_column_pkey
  FOREIGN KEY (id1_fkey, id2_fkey) REFERENCES multi_column_pkey (id1, id2);

INSERT INTO avatars
    (id, url)
  VALUES
    ('09af5166-a1ed-11ef-b864-0242ac120002', 'first.png'),
    ('0cf7650c-a1ed-11ef-b864-0242ac120002', 'second.png'),
    ('184e5e10-a1ed-11ef-b864-0242ac120002', 'third.png');

INSERT INTO avatar_modifiers
    (id, grayscale)
  VALUES
    ('09af5166-a1ed-11ef-b864-0242ac120002', FALSE),
    ('0cf7650c-a1ed-11ef-b864-0242ac120002', TRUE),
    ('184e5e10-a1ed-11ef-b864-0242ac120002', FALSE);

INSERT INTO roles
    (id, name)
  VALUES
    ('d1e36a3c-32ca-4b26-9358-ab117b685aaf', 'Boss'),
    ('17b0b806-a907-4097-a86e-d5a2e44a55b0', 'Mini Boss'),
    ('65906a7e-877c-4e39-acc2-f2accd1495f1', 'Minion');

INSERT INTO employees
    (id, role)
  VALUES
    (1, 'Boss'),
    (2, 'Mini Boss'),
    (3, 'Minion');

INSERT INTO users
    (id, avatar_id, email, employee_id)
  VALUES
    ('448e6222-a1ed-11ef-b864-0242ac120002', '09af5166-a1ed-11ef-b864-0242ac120002', 'first@example.com', 1),
    ('459a966e-a1f1-11ef-b864-0242ac120002', '0cf7650c-a1ed-11ef-b864-0242ac120002', 'second@example.com', 2),
    ('4848cf02-a1f1-11ef-b864-0242ac120002', '184e5e10-a1ed-11ef-b864-0242ac120002', 'third@example.com', 3);

INSERT INTO multi_column_pkey
    (id1, id2)
  VALUES
    ('0a794e82-ed63-11ef-9cd2-0242ac120002', '6d5c2f60-ed63-11ef-9cd2-0242ac120002');

INSERT INTO multi_column_fkey
    (id, id1_fkey, id2_fkey)
  VALUES
    ('737ba6d2-ed63-11ef-9cd2-0242ac120002', '0a794e82-ed63-11ef-9cd2-0242ac120002', '6d5c2f60-ed63-11ef-9cd2-0242ac120002');
