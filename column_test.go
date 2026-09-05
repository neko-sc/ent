package ent_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	builder "github.com/neko-sc/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

type columnUser struct{}

type columnPet struct{}

var (
	userName     = ent.StringColumn[columnUser, string]{OrderedColumn: ent.OrderedColumn[columnUser, string]{Column: ent.Column[columnUser, string]{ColumnRef: ent.ColumnRef{Name: "name"}}}}
	userAge      = ent.OrderedColumn[columnUser, int]{Column: ent.Column[columnUser, int]{ColumnRef: ent.ColumnRef{Name: "age"}}}
	userTags     = ent.ArrayColumn[columnUser, []string]{Column: ent.Column[columnUser, []string]{ColumnRef: ent.ColumnRef{Name: "tags"}}}
	userDocument = ent.JSONColumn[columnUser, map[string]any]{Column: ent.Column[columnUser, map[string]any]{ColumnRef: ent.ColumnRef{Name: "document"}}}
)

// Set deliberately takes the column interface, not its embedded base column.
func Set[E, T any](column ent.ColumnOf[E, T], value T) T { return value }

func inferredColumnValue[E, T any](column ent.ColumnOf[E, T]) T {
	var zero T
	return zero
}

func TestColumnInference(t *testing.T) {
	require.Equal(t, "x", Set(userName, "x"))
	var inferred string = inferredColumnValue(userName)
	require.Empty(t, inferred)
	require.Equal(t, 3, Set(userAge, 3))
	require.Equal(t, []string{"a"}, Set(userTags, []string{"a"}))
	var _ ent.ColumnOf[columnUser, string] = userName
	var _ ent.ColumnOf[columnUser, int] = userAge
	var _ ent.ColumnOf[columnUser, []string] = userTags
	var _ ent.ColumnOf[columnUser, map[string]any] = userDocument
	var _ ent.ColumnOf[columnUser, bool] = ent.Column[columnUser, bool]{}
	var _ ent.ColumnOf[columnUser, int] = ent.Column[columnUser, int]{}
	var _ ent.SelectionOf[string] = userName
}

func TestColumnPredicates(t *testing.T) {
	for _, test := range []struct {
		name      string
		predicate ent.Predicate[columnUser]
		where     string
		arguments []any
	}{
		{"equal", userName.EQ("a"), `"users"."name" = $1`, []any{"a"}},
		{"not equal", userName.NEQ("a"), `"users"."name" <> $1`, []any{"a"}},
		{"in", userAge.In(1, 2), `"users"."age" IN ($1, $2)`, []any{1, 2}},
		{"not in", userAge.NotIn(1, 2), `"users"."age" NOT IN ($1, $2)`, []any{1, 2}},
		{"empty in", userAge.In(), `FALSE`, nil},
		{"empty not in", userAge.NotIn(), `NOT (FALSE)`, nil},
		{"null", userName.IsNull(), `"users"."name" IS NULL`, nil},
		{"not null", userName.NotNull(), `"users"."name" IS NOT NULL`, nil},
		{"greater", userAge.GT(1), `"users"."age" > $1`, []any{1}},
		{"greater equal", userAge.GTE(1), `"users"."age" >= $1`, []any{1}},
		{"less", userAge.LT(1), `"users"."age" < $1`, []any{1}},
		{"less equal", userAge.LTE(1), `"users"."age" <= $1`, []any{1}},
		{"between", userAge.Between(1, 9), `"users"."age" >= $1 AND "users"."age" <= $2`, []any{1, 9}},
		{"contains", userName.Contains("a"), `"users"."name" LIKE $1`, []any{"%a%"}},
		{"prefix", userName.HasPrefix("a"), `"users"."name" LIKE $1`, []any{"a%"}},
		{"suffix", userName.HasSuffix("a"), `"users"."name" LIKE $1`, []any{"%a"}},
		{"equal fold", userName.EqualFold("A"), `"users"."name" ILIKE $1`, []any{"a"}},
		{"contains fold", userName.ContainsFold("A"), `"users"."name" ILIKE $1`, []any{"%a%"}},
		{"equal expression", userAge.EQExpr(ent.Add(userAge.Expr(), ent.Literal(1))), `"users"."age" = ("users"."age" + $1)`, []any{1}},
		{"not equal expression", userAge.NEQExpr(ent.Excluded(userAge)), `"users"."age" <> "excluded"."age"`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := builder.Dialect(dialect.Postgres).Select("*").From(builder.Table("users"))
			test.predicate(selector)
			query, arguments := selector.Query()
			require.NoError(t, selector.Err())
			require.Equal(t, `SELECT * FROM "users" WHERE `+test.where, query)
			require.Equal(t, test.arguments, arguments)
		})
	}
}

func TestPredicateCompositionAndAlias(t *testing.T) {
	for _, dialectName := range []string{dialect.Postgres, dialect.SQLite} {
		t.Run(dialectName, func(t *testing.T) {
			selector := builder.Dialect(dialectName).Select("*").From(builder.Table("users").As("u"))
			userAge.GTE(18)(selector)
			ent.Or(userName.EQ("a"), ent.And(userName.EQ("b"), ent.Not(userAge.GT(50))))(selector)
			ent.And[columnUser]()(selector)
			ent.Or[columnUser]()(selector)
			query, arguments := selector.Query()
			require.Equal(t, []any{18, "a", "b", 50}, arguments)
			if dialectName == dialect.Postgres {
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "u"."age" >= $1 AND ("u"."name" = $2 OR ("u"."name" = $3 AND (NOT ("u"."age" > $4))))`, query)
			} else {
				require.Equal(t, "SELECT * FROM `users` AS `u` WHERE `u`.`age` >= ? AND (`u`.`name` = ? OR (`u`.`name` = ? AND (NOT (`u`.`age` > ?))))", query)
			}
			joined := ent.Column[columnPet, string]{ColumnRef: ent.ColumnRef{Table: "p", Name: "name"}}
			selector = builder.Dialect(dialectName).Select("*").From(builder.Table("users").As("u"))
			userName.EQExpr(joined.Expr())(selector)
			joined.Desc(builder.OrderNullsFirst())(selector)
			userAge.Asc(builder.OrderNullsLast())(selector)
			query, arguments = selector.Query()
			require.Empty(t, arguments)
			if dialectName == dialect.Postgres {
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "u"."name" = "p"."name" ORDER BY "p"."name" DESC NULLS FIRST, "u"."age" ASC NULLS LAST`, query)
			} else {
				require.Equal(t, "SELECT * FROM `users` AS `u` WHERE `u`.`name` = `p`.`name` ORDER BY `p`.`name` DESC NULLS FIRST, `u`.`age` ASC NULLS LAST", query)
			}
		})
	}
}

func TestColumnValuer(t *testing.T) {
	conversionError := errors.New("invalid stored value")
	column := ent.OrderedColumn[columnUser, int]{Table: "users", Name: "age", Valuer: func(value int) (driver.Value, error) {
		if value < 0 {
			return nil, conversionError
		}
		return int64(value * 2), nil
	}}
	for _, predicate := range []ent.Predicate[columnUser]{column.EQ(3), column.NEQ(3), column.GT(3), column.GTE(3), column.LT(3), column.LTE(3), column.In(3), column.NotIn(3)} {
		selector := builder.Dialect(dialect.Postgres).Select().From(builder.Table("users"))
		predicate(selector)
		_, arguments := selector.Query()
		require.NoError(t, selector.Err())
		require.Equal(t, []any{int64(6)}, arguments)
	}
	for _, predicate := range []ent.Predicate[columnUser]{column.EQ(-1), column.GT(-1), column.In(3, -1), column.NotIn(-1)} {
		selector := builder.Dialect(dialect.Postgres).Select().From(builder.Table("users"))
		predicate(selector)
		require.ErrorContains(t, selector.Err(), conversionError.Error())
	}
	text := ent.StringColumn[columnUser, string]{Table: "users", Name: "name", Valuer: func(value string) (driver.Value, error) { return "encoded:" + value, nil }}
	predicate := text.HasPrefix("a")
	for range 2 {
		selector := builder.Dialect(dialect.Postgres).Select().From(builder.Table("users"))
		predicate(selector)
		_, arguments := selector.Query()
		require.Equal(t, []any{"encoded:a%"}, arguments)
	}
}

func TestPredicateMethods(t *testing.T) {
	selector := builder.Dialect(dialect.Postgres).Select().From(builder.Table("users"))
	userAge.GTE(18).And(userName.EQ("a").Or(userName.EQ("b"))).Not()(selector)
	query, arguments := selector.Query()
	require.Equal(t, `SELECT * FROM "users" WHERE NOT ("users"."age" >= $1 AND ("users"."name" = $2 OR "users"."name" = $3))`, query)
	require.Equal(t, []any{18, "a", "b"}, arguments)
}

func TestArrayPostgresRendering(t *testing.T) {
	for _, test := range []struct {
		name      string
		predicate ent.Predicate[columnUser]
		where     string
		arguments []any
	}{
		{"contains", userTags.Contains("a"), `"users"."tags" @> $1`, []any{[]any{"a"}}},
		{"any", userTags.HasAny("a", "b"), `"users"."tags" && $1`, []any{[]any{"a", "b"}}},
		{"all", userTags.HasAll("a", "b"), `"users"."tags" @> $1`, []any{[]any{"a", "b"}}},
		{"empty any", userTags.HasAny(), `"users"."tags" && $1`, []any{[]any{}}},
		{"empty all", userTags.HasAll(), `"users"."tags" @> $1`, []any{[]any{}}},
		{"length equal", userTags.LenEQ(2), `cardinality("users"."tags") = $1`, []any{2}},
		{"length unequal", userTags.LenNEQ(2), `cardinality("users"."tags") <> $1`, []any{2}},
		{"length greater", userTags.LenGT(2), `cardinality("users"."tags") > $1`, []any{2}},
		{"length greater equal", userTags.LenGTE(2), `cardinality("users"."tags") >= $1`, []any{2}},
		{"length less", userTags.LenLT(2), `cardinality("users"."tags") < $1`, []any{2}},
		{"length less equal", userTags.LenLTE(2), `cardinality("users"."tags") <= $1`, []any{2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := builder.Dialect(dialect.Postgres).Select("*").From(builder.Table("users"))
			test.predicate(selector)
			query, arguments := selector.Query()
			require.Equal(t, `SELECT * FROM "users" WHERE `+test.where, query)
			require.Equal(t, test.arguments, arguments)
		})
	}
}

func TestJSONPostgresRendering(t *testing.T) {
	for _, test := range []struct {
		name      string
		predicate ent.Predicate[columnUser]
		where     string
		arguments []any
	}{
		{"string", userDocument.Path("profile", "name").EQ("Alice"), `"users"."document"->'profile'->>'name' = $1`, []any{"Alice"}},
		{"quoted key", userDocument.Path("user's name").EQ("Alice"), `"users"."document"->>'user''s name' = $1`, []any{"Alice"}},
		{"number", userDocument.Path("number").NEQ(3), `("users"."document"->>'number')::int <> $1`, []any{3}},
		{"null", userDocument.Path("nil").IsNull(), `("users"."document"->'nil')::jsonb = 'null'::jsonb`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := builder.Dialect(dialect.Postgres).Select("*").From(builder.Table("users"))
			test.predicate(selector)
			query, arguments := selector.Query()
			require.Equal(t, `SELECT * FROM "users" WHERE `+test.where, query)
			require.Equal(t, test.arguments, arguments)
		})
	}
}

func TestNillableColumnEquality(t *testing.T) {
	column := ent.Column[columnUser, *int]{ColumnRef: ent.ColumnRef{Name: "age"}}
	selector := builder.Dialect(dialect.Postgres).Select("*").From(builder.Table("users"))
	column.EQ(nil)(selector)
	query, arguments := selector.Query()
	require.Equal(t, `SELECT * FROM "users" WHERE "users"."age" = $1`, query)
	require.Equal(t, []any{(*int)(nil)}, arguments)
}

func TestSQLiteArrayAndJSONPredicates(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE users (id INTEGER, tags JSON, document JSON);
	INSERT INTO users VALUES (1, '["a","b"]', '{"name":"alpha","number":3,"nil":null,"user''s name":"Alice"}'),
	(2, '["b"]', '{"name":"beta","number":4}'), (3, '[]', '{}'), (4, NULL, NULL);`)
	require.NoError(t, err)
	for _, test := range []struct {
		name      string
		predicate ent.Predicate[columnUser]
		ids       []int
	}{
		{"contains", userTags.Contains("a"), []int{1}},
		{"any", userTags.HasAny("a", "b"), []int{1, 2}},
		{"all", userTags.HasAll("a", "b"), []int{1}},
		{"duplicates", userTags.HasAll("b", "b"), []int{1, 2}},
		{"empty any", userTags.HasAny(), nil},
		{"empty all", userTags.HasAll(), []int{1, 2, 3}},
		{"negated contains excludes SQL NULL", ent.Not(userTags.Contains("a")), []int{2, 3}},
		{"negated empty any excludes SQL NULL", ent.Not(userTags.HasAny()), []int{1, 2, 3}},
		{"null member", userTags.Contains(nil), nil},
		{"length", userTags.LenEQ(0), []int{3}},
		{"length comparison", userTags.LenGTE(1), []int{1, 2}},
		{"json equality", userDocument.Path("name").EQ("alpha"), []int{1}},
		{"json quoted key", userDocument.Path("user's name").EQ("Alice"), []int{1}},
		{"json inequality", userDocument.Path("number").NEQ(3), []int{2}},
		{"json null", userDocument.Path("nil").IsNull(), []int{1}},
		{"document SQL NULL", userDocument.IsNull(), []int{4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := builder.Dialect(dialect.SQLite).Select("id").From(builder.Table("users"))
			test.predicate(selector)
			query, arguments := selector.Query()
			require.NoError(t, selector.Err())
			rows, err := db.Query(query, arguments...)
			require.NoError(t, err, query)
			defer rows.Close()
			var ids []int
			for rows.Next() {
				var id int
				require.NoError(t, rows.Scan(&id))
				ids = append(ids, id)
			}
			require.NoError(t, rows.Err())
			require.Equal(t, test.ids, ids)
		})
	}
}
