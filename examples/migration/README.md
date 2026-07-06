# Versioned Migration Example

Example for generating versioned migration files with Atlas and Ent.

### Migration directory

Versioned migration files exists under `ent/migrate/migrations`.

### Changes to the Ent schema

1\. Change the `ent/schema`.

2\. Run `go generate ./ent`

### Generate a new migration file

```bash
atlas migrate diff <migration_name> \
  --dir "file://ent/migrate/migrations" \
  --to "ent://ent/schema" \
  --dev-url "docker://postgres/16/ent"
```

### Run migration linting

```bash
atlas migrate lint \
  --dev-url="docker://postgres/16/dev" \
  --dir="file://ent/migrate/migrations" \
  --latest=1
```
