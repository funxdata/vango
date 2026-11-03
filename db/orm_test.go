package db

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

type testUser struct {
	ID    int64  `db:"id,auto"`
	Name  string `db:"name"`
	Email string `json:"email"`
	Note  string `db:"-"`
}

type recordingExecer struct {
	query string
	args  []any
}

func (r *recordingExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	r.query = query
	r.args = args
	return nil, nil
}

func TestCollectColumns(t *testing.T) {
	columns := collectColumns(reflect.TypeOf(testUser{}))

	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.name)
	}

	want := []string{"id", "name", "email"}
	if len(names) != len(want) {
		t.Fatalf("columns = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("columns = %v, want %v", names, want)
		}
	}

	if !columns[0].auto {
		t.Fatal("id column should be marked auto")
	}
}

func TestToSnake(t *testing.T) {
	cases := map[string]string{
		"ID":         "i_d",
		"Name":       "name",
		"UserName":   "user_name",
		"TenantID":   "tenant_i_d",
		"lower_case": "lower_case",
	}
	for input, want := range cases {
		if got := toSnake(input); got != want {
			t.Fatalf("toSnake(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInsertBuildsSQL(t *testing.T) {
	exec := &recordingExecer{}
	user := testUser{Name: "vango", Email: "hi@example.com"}

	if _, err := Insert(context.Background(), exec, "users", user); err != nil {
		t.Fatalf("Insert returned error: %v", err)
	}

	const want = "INSERT INTO users (name, email) VALUES (?, ?)"
	if exec.query != want {
		t.Fatalf("query = %q, want %q", exec.query, want)
	}
	if len(exec.args) != 2 {
		t.Fatalf("args = %v, want 2 values", exec.args)
	}
	if exec.args[0] != "vango" || exec.args[1] != "hi@example.com" {
		t.Fatalf("unexpected args %v", exec.args)
	}
}

func TestInsertDollarDialect(t *testing.T) {
	exec := &recordingExecer{}
	if _, err := Insert(context.Background(), exec, "users", testUser{Name: "a"}, WithDialect(Dollar)); err != nil {
		t.Fatalf("Insert returned error: %v", err)
	}
	if exec.query != "INSERT INTO users (name, email) VALUES ($1, $2)" {
		t.Fatalf("query = %q", exec.query)
	}
}

func TestInsertRejectsInvalidTable(t *testing.T) {
	exec := &recordingExecer{}
	if _, err := Insert(context.Background(), exec, "users; drop table users", testUser{}); err == nil {
		t.Fatal("expected invalid identifier error")
	}
}

func TestUpdateBuildsSQL(t *testing.T) {
	exec := &recordingExecer{}
	user := testUser{ID: 7, Name: "vango", Email: "hi@example.com"}

	if _, err := Update(context.Background(), exec, "users", user, "id = ?", user.ID); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	const want = "UPDATE users SET name = ?, email = ? WHERE id = ?"
	if exec.query != want {
		t.Fatalf("query = %q, want %q", exec.query, want)
	}
	if len(exec.args) != 3 || exec.args[2] != int64(7) {
		t.Fatalf("unexpected args %v", exec.args)
	}
}

func TestUpdateRequiresWhere(t *testing.T) {
	exec := &recordingExecer{}
	if _, err := Update(context.Background(), exec, "users", testUser{}, "  "); err == nil {
		t.Fatal("expected error for empty where clause")
	}
}

func TestDeleteRequiresWhere(t *testing.T) {
	exec := &recordingExecer{}
	if _, err := Delete(context.Background(), exec, "users", ""); err == nil {
		t.Fatal("expected error for empty where clause")
	}
}

func TestUpdateMapBuildsSQL(t *testing.T) {
	exec := &recordingExecer{}
	data := map[string]any{"name": "vango", "email": "hi@example.com"}

	if _, err := UpdateMap(context.Background(), exec, "users", data, "id = ?", 1); err != nil {
		t.Fatalf("UpdateMap returned error: %v", err)
	}
	if !strings.HasPrefix(exec.query, "UPDATE users SET email = ?, name = ?") {
		t.Fatalf("query = %q", exec.query)
	}
	if len(exec.args) != 3 {
		t.Fatalf("args = %v, want 3 values", exec.args)
	}
}

func TestDialectFor(t *testing.T) {
	if DialectFor("postgres") != Dollar {
		t.Fatal("postgres should use $n placeholders")
	}
	if DialectFor("mysql") != Question {
		t.Fatal("mysql should use ? placeholders")
	}
}

func TestStructValueOfRejectsNonStruct(t *testing.T) {
	if _, err := structValueOf(42); err == nil {
		t.Fatal("expected error for non-struct value")
	}
	if _, err := structValueOf((*testUser)(nil)); err == nil {
		t.Fatal("expected error for nil pointer")
	}
}

func TestUpdateMapDollarDialect(t *testing.T) {
	exec := &recordingExecer{}
	_, err := UpdateMapWithOptions(context.Background(), exec, "users",
		map[string]any{"name": "vango"}, "id = $2", []any{1}, []CRUDOption{WithDialect(Dollar)})
	if err != nil {
		t.Fatalf("UpdateMapWithOptions returned error: %v", err)
	}
	if exec.query != "UPDATE users SET name = $1 WHERE id = $2" {
		t.Fatalf("query = %q", exec.query)
	}
}
