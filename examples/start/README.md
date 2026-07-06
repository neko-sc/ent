# Getting Started Example

A getting-started example for Ent.

### Generate Assets

```console
go generate ./...
```

### Generate Migration Files

```console
atlas migrate diff migration_name \
  --dir "file://ent/migrate/migrations" \
  --to "ent://ent/schema" \
  --dev-url "sqlite://file?mode=memory&_fk=1"
```
