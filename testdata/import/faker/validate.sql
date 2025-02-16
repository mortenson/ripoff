WITH test AS (
  SELECT count(*) as count FROM users
  WHERE email = 'nelsonyost@russel.biz'
  AND id = '6b30cfb0-a35b-4584-a035-1334515f846b'
  AND date_trunc('day', last_logged_in_at) = date_trunc('day', now() - interval '10 days')
)
SELECT (select count from test),'email: ' || users.email || ' id: ' || users.id || ' last_logged_in_at: ' || users.last_logged_in_at
FROM users;
