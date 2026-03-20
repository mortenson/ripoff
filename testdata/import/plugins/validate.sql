WITH test AS (
  SELECT count(*) as count FROM users
  WHERE (email = 'foobar@example.com' AND name = 'Hello World')
  OR (email = 'smorty@example.com' AND name = 'Goodbye Moon')
)
SELECT (select count from test) = 2,'email: ' || string_agg(users.email, ',') || ' name: ' || string_agg(users.name, ',')
FROM users;
