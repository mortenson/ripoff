# ripoff - generate fake data from templated yaml files

ripoff is a command line tool that generates fake data from yaml files (ripoffs) and inserts that data into PostgreSQL.

Some features of ripoff are:

- Model your fake data in one or more yaml files, god's favorite file format
- Provide templated yaml files in cases where a row in one table requires many other rows, or you want loops
- Due to deterministic random generation, re-running ripoff will perform upserts on all generated rows

# Installation

1. Run `go install github.com/mortenson/ripoff/cmd/ripoff@latest`
2. Set the `DATABASE_URL` env variable to your local PostgreSQL database
3. Run `ripoff <directory to your yaml files>`

When writing your initial ripoffs, you may want to use the `-s` ("soft") flag which does not commit the generated transaction.

# File format

ripoffs define rows to be inserted into your database. Any number of ripoffs can be included in a single directory.

## Basic example

```yaml
# A map of rows to upsert, where keys are in the format <table_name>:<valueFunc>(<seed>),
# and values are maps of column names to values.
rows:
  # A "users" table row identified with a UUID generated with the seed "fooBar"
  users:uuid(fooBar):
    email: foobar@example.com
    # Note that ripoff will automatically set primary key columns, so you don't need to add:
    # id: users:uuid(fooBar)
  avatars:uuid(fooBarAvatar):
    # ripoff will see this and insert the "users:uuid(fooBar)" row before this row
    user_id: users:uuid(fooBar)
  users:uuid(randomUser):
    # Generate a random email with the seed "randomUser"
    email: email(randomUser)
```

For more (sometimes wildly complex) examples, see `./testdata`.

## More on valueFuncs and row keys

valueFuncs allow you to generate random data that's seeded with a static string. This ensures that repeat runs of ripoff are deterministic, which enables upserts (consistent primary keys).

ripoff provides:

- `uuid(seedString)` - generates a v1 UUID
- `int(seedString)` - generates an integer (note: might be awkward on auto incrementing tables)

and also all functions from [gofakeit](https://github.com/brianvoe/gofakeit?tab=readme-ov-file#functions) that have no arguments and return a string (called in camelcase, ex: `email(seedString)`). For the full list, see `./gofakeit.go`.

## Using templates

ripoff files can be used as templates to create multiple rows at once.

Yaml files that start with `template_` will be treated as Go templates. Here's a template that creates a user and an avatar:

```yaml
rows:
  # "rowId" is the id/key of the row that rendered this template.
  # You could also pass an explicit seed and use it like `users:uuid({{ .seed }})`
  {{ .rowId }}:
    email: {{ .email }}
    # It's convenient to use the rowId as a seed to other valueFuncs.
    avatar_id: avatars:uuid({{ .rowId }})
  avatars:uuid({{ .rowId }}):
    url: {{ .avatarUrl }}
```

which you would use from a "normal" ripoff like:

```yaml
rows:
  # The row id/key will be passed to the template in the "rowId" variable.
  # This is useful if you'd like to reference "users:uuid(fooBar)" in a foreign key elsewhere.
  users:uuid(fooBar):
    # The template filename.
    template: template_user.yml
    # All other variables are arbitrary.
    email: foobar@example.com
    avatarUrl: image.png
    avatarGrayscale: false
```

### Special template variables

- `rowId` - The map key of the row using this template, ex `users:uuid(fooBar)`. Useful for allowing the "caller" to provide their own ID for the "main" row being created, if there is one. Optional to use if you find it awkward.
- `enums` - A map of SQL enums names to an array of enum values. Useful for creating one row for each value of an enum (ex: each user role).

# Security

This project explicitly allows SQL injection due to the way queries are constructed. Do not run `ripoff` on directories you do not trust.

# Why this exists

Fake data generators generally come in two flavors:

1. Model your fake data in the same language/DSL/ORM that your application uses
2. Fuzz your database schema by spewing completely randomized data at it

I find generating fake data to be a completely separate use case from "normal" ORM usage, and truly randomized fake data is awkward to use locally.

So ripoff is my approach to fake (but not excessively random) data generation. Since it's not aware of your application or schema, it's closer to writing templated SQL than learning some crazy high level DSL.

# FAQ

## Why use Go templates and valueFuncs?

It's kind of weird that `template_*` files use Go templates, but there's also valueFuncs like `uuid(someSeed)`.

This is done for two reasons - first, Go templates are ugly and result in invalid yaml, so no reason to force you to write them unless you need to. Second, ripoff builds its dependency graph based on the row ids/keys, not the actual generated random value. So you can think of the query building pipeline as:

1. Load all ripoff files
2. For each row in each file, check if the row uses a template
3. If it does, process the template and append the templated rows into the total rows
4. If not, just append that row
5. Now we have a "total ripoff" (har har) file which contains all rows. I think it's cool at this point that the templating is "done"
6. For each row, check if any column references another row and build a directed acyclic graph, then sort that graph
7. Run queries for each row, in order of least to greatest dependencies
