package db

import (
	"context"
	"database/sql"
	"fmt"
)

// QueryAll 执行查询并把每一行映射成 T。
//
//	users, err := db.QueryAll[User](ctx, database,
//		"SELECT id, name FROM users WHERE tenant_id = $1", tenantID)
func QueryAll[T any](ctx context.Context, queryer Queryer, query string, args ...any) ([]T, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: query: %w", err)
	}
	return ScanAll[T](rows)
}

// QueryOne 执行单行查询，没有结果时返回 (nil, nil)。
func QueryOne[T any](ctx context.Context, queryer Queryer, query string, args ...any) (*T, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: query one: %w", err)
	}
	return ScanOne[T](rows)
}

// Exec 执行写入语句，是 ExecContext 的简写。
func Exec(ctx context.Context, execer Execer, query string, args ...any) (sql.Result, error) {
	return execer.ExecContext(ctx, query, args...)
}
