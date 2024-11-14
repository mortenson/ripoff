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
    (gen_random_uuid(), 'Boss'),
    (gen_random_uuid(), 'Mini Boss'),
    (gen_random_uuid(), 'Minion');

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
