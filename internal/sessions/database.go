package sessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func openDatabase(ctx context.Context, path string, writable bool) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	p := filepath.ToSlash(path)
	if len(p) > 1 && p[1] == ':' {
		p = "/" + p
	}
	mode := "ro"
	if writable {
		mode = "rw"
	}
	u := url.URL{Scheme: "file", Path: p}
	q := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(3000)", "foreign_keys(ON)"}, "_txlock": {"immediate"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func quoteSQL(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

type sqlReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func databaseTables(ctx context.Context, db sqlReader) (map[string][]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND sql NOT LIKE 'CREATE VIRTUAL%'`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := map[string][]string{}
	for _, name := range names {
		r, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteSQL(name)+")")
		if err != nil {
			return nil, err
		}
		for r.Next() {
			var cid, notnull, pk int
			var column, typ string
			var def any
			if err := r.Scan(&cid, &column, &typ, &notnull, &def, &pk); err != nil {
				r.Close()
				return nil, err
			}
			out[name] = append(out[name], column)
		}
		if err := r.Err(); err != nil {
			r.Close()
			return nil, err
		}
		r.Close()
	}
	return out, nil
}

func columnExpr(columns []string, name, fallback string) string {
	for _, column := range columns {
		if column == name {
			return "COALESCE(" + quoteSQL(name) + "," + fallback + ")"
		}
	}
	return fallback
}

func missingStore(err error) bool { return errors.Is(err, os.ErrNotExist) }

func unsupportedStore(provider string) error {
	return fmt.Errorf("%s 数据库结构需要适配，请检查工具版本", provider)
}
