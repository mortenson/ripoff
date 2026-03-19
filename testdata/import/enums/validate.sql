WITH test AS (
  SELECT array_agg(distinct role) as roles FROM users
)
-- db_test.go will automatically determine that the correct number of rows
-- were inserted, but in this case we want to make sure every users row also
-- has a distinct user role.
SELECT array_length(roles, 1) = 4,roles
FROM test;
