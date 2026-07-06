## Multi-schema support for versioned migration

Login first:

```shell
atlas login
```

Generate migrations:

```shell
atlas migrate diff --to ent://versioned/schema \
  --dev-url docker://postgres/16 \
  --format '{{ sql . "  " }}'
```

Apply migrations:

```shell
atlas migrate apply \
  --url postgres://postgres:pass@localhost:5436/test?sslmode=disable \
  --dir file://versioned/migrate/migrations
```

Inspect/Visualize the schema:

```shell
atlas schema inspect \
  --url ent://versioned/schema \
  --dev-url docker://postgres/16 \
  -w
```
