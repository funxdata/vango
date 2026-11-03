// Package db 是 database/sql 的薄封装，提供连接管理、事务便捷方法与轻量 ORM 能力。
//
// vango 不隐藏 database/sql：DB 内嵌 *sql.DB，复杂查询、驱动特性、连接池都仍然可用。
//
//	d, err := db.Open("sqlite", ":memory:",
//		db.WithMaxOpenConns(1),
//	)
//	if err != nil { ... }
//	defer d.Close()
//
//	// 轻量 ORM：结构体插入
//	_, err = db.Insert(ctx, d, "users", User{Name: "vango"})
//
//	// 查询 + 自动映射
//	rows, err := d.QueryContext(ctx, "SELECT id, name FROM users")
//	users, err := db.ScanAll[User](rows)
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DB 是对 *sql.DB 的封装。
type DB struct {
	*sql.DB

	driver string
	dsn    string
}

// Option 定制连接池参数。
type Option func(*DB)

// WithMaxOpenConns 设置最大打开连接数，默认 25。
func WithMaxOpenConns(n int) Option {
	return func(d *DB) { d.DB.SetMaxOpenConns(n) }
}

// WithMaxIdleConns 设置最大空闲连接数，默认 25。
func WithMaxIdleConns(n int) Option {
	return func(d *DB) { d.DB.SetMaxIdleConns(n) }
}

// WithConnMaxLifetime 设置连接最长存活时间，默认 30 分钟。
func WithConnMaxLifetime(duration time.Duration) Option {
	return func(d *DB) { d.DB.SetConnMaxLifetime(duration) }
}

// WithConnMaxIdleTime 设置连接最长空闲时间。
func WithConnMaxIdleTime(duration time.Duration) Option {
	return func(d *DB) { d.DB.SetConnMaxIdleTime(duration) }
}

// Open 打开数据库并应用连接池默认值。
//
// 驱动需要由使用方通过 blank import 注册，例如：
//
//	import _ "github.com/mattn/go-sqlite3"
func Open(driverName, dsn string, opts ...Option) (*DB, error) {
	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", driverName, err)
	}

	d := &DB{DB: sqlDB, driver: driverName, dsn: dsn}
	d.SetMaxOpenConns(25)
	d.SetMaxIdleConns(25)
	d.SetConnMaxLifetime(30 * time.Minute)

	for _, opt := range opts {
		opt(d)
	}

	if err := d.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: ping %s: %w", driverName, err)
	}
	return d, nil
}

// Driver 返回驱动名。
func (d *DB) Driver() string { return d.driver }

// DSN 返回数据源名称。
func (d *DB) DSN() string { return d.dsn }

// Tx 在事务中执行 fn：fn 返回错误时回滚，否则提交。
func (d *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Execer 抽象 *sql.DB 与 *sql.Tx，让 CRUD 辅助函数可以同时用于两者。
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Queryer 抽象查询能力。
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Querier 同时具备执行与查询能力。
type Querier interface {
	Execer
	Queryer
}

// QueryRower 抽象单行查询，*sql.DB 与 *sql.Tx 均满足。
type QueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
