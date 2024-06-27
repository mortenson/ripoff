WITH test AS (
  SELECT count(*) as count FROM users
  WHERE email = 'nelsonyost@russel.biz'
  AND id = '6b30cfb0-a35b-4584-a035-1334515f846b'
)
SELECT (select count from test),'email: ' || users.email || ' id: ' || users.id
FROM users;
