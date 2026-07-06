### entc integration tests

#### Regenerating assets

If you edited one of the files in `entc/gen/template` or `entc/load/template`,
run the following command:

```
go generate ./...
```

#### Running the integration tests

```
docker compose up -d
go test .
```

Use the `-run` flag for running specific test or set of tests. For example:

```
go test -run=Postgres

go test -run=Postgres/16/Sanity

go test -run=SQLite/Sanity
```
