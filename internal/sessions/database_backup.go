package sessions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type SQLValue struct {
	Kind    string  `json:"kind"`
	Integer int64   `json:"integer,omitempty"`
	Real    float64 `json:"real,omitempty"`
	Text    string  `json:"text,omitempty"`
	Blob    []byte  `json:"blob,omitempty"`
}

func sqlValue(value any) SQLValue {
	switch v := value.(type) {
	case nil:
		return SQLValue{Kind: "null"}
	case int64:
		return SQLValue{Kind: "integer", Integer: v}
	case float64:
		return SQLValue{Kind: "real", Real: v}
	case string:
		return SQLValue{Kind: "text", Text: v}
	case []byte:
		return SQLValue{Kind: "blob", Blob: append([]byte{}, v...)}
	}
	return SQLValue{Kind: "null"}
}

func (v SQLValue) value() any {
	switch v.Kind {
	case "integer":
		return v.Integer
	case "real":
		return v.Real
	case "text":
		return v.Text
	case "blob":
		if v.Blob == nil {
			return []byte{}
		}
		return v.Blob
	}
	return nil
}

type TableBackup struct {
	Name    string       `json:"name"`
	Columns []string     `json:"columns"`
	Primary []string     `json:"primary"`
	Rows    [][]SQLValue `json:"rows"`
}

type DatabaseBackup struct {
	Path   string        `json:"path"`
	Tables []TableBackup `json:"tables"`
}

type databaseChange struct {
	db        *sql.DB
	tx        *sql.Tx
	backup    DatabaseBackup
	committed bool
}

func tablePrimary(ctx context.Context, tx *sql.Tx, name string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+quoteSQL(name)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		position int
		name     string
	}
	var columns []item
	for rows.Next() {
		var cid, notnull, pk int
		var column, typ string
		var def any
		if err := rows.Scan(&cid, &column, &typ, &notnull, &def, &pk); err != nil {
			return nil, err
		}
		if pk > 0 {
			columns = append(columns, item{pk, column})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(columns, func(a, b item) int { return a.position - b.position })
	var out []string
	for _, c := range columns {
		out = append(out, c.name)
	}
	return out, nil
}

func readSQLRows(ctx context.Context, tx *sql.Tx, table, where string, args ...any) ([]string, [][]SQLValue, error) {
	rows, err := tx.QueryContext(ctx, "SELECT * FROM "+quoteSQL(table)+" WHERE "+where, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out [][]SQLValue
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make([]SQLValue, len(columns))
		for i, v := range values {
			row[i] = sqlValue(v)
		}
		out = append(out, row)
	}
	return columns, out, rows.Err()
}

func captureDatabase(ctx context.Context, path, provider string, ids []string) (*databaseChange, error) {
	db, err := openDatabase(ctx, path, true)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, err
	}
	change := &databaseChange{db: db, tx: tx, backup: DatabaseBackup{Path: path}}
	ok := false
	defer func() {
		if !ok {
			tx.Rollback()
			db.Close()
		}
	}()
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys=ON"); err != nil {
		return nil, err
	}
	tables, err := databaseTables(ctx, tx)
	if err != nil {
		return nil, err
	}
	backups := map[string]*TableBackup{}
	seen := map[string]map[string]bool{}
	collect := func(table, where string, args ...any) (bool, error) {
		columns, rows, err := readSQLRows(ctx, tx, table, where, args...)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		backup := backups[table]
		if backup == nil {
			pk, err := tablePrimary(ctx, tx, table)
			if err != nil {
				return false, err
			}
			if len(pk) == 0 {
				return false, fmt.Errorf("数据表 %s 缺少主键，需要适配后再删除", table)
			}
			backup = &TableBackup{Name: table, Columns: columns, Primary: pk}
			backups[table] = backup
			seen[table] = map[string]bool{}
		}
		added := false
		for _, row := range rows {
			var key []SQLValue
			for _, name := range backup.Primary {
				index := slices.Index(columns, name)
				if index < 0 || row[index].Kind == "null" {
					return false, errors.New("会话记录的主键无效")
				}
				key = append(key, row[index])
			}
			raw, _ := json.Marshal(key)
			if !seen[table][string(raw)] {
				seen[table][string(raw)] = true
				backup.Rows = append(backup.Rows, row)
				added = true
			}
		}
		return added, nil
	}
	encoded, _ := json.Marshal(ids)
	for table, columns := range tables {
		var clauses []string
		switch provider {
		case "opencode":
			if table == "session" || table == "session_v2" {
				clauses = append(clauses, `id IN (SELECT value FROM json_each(?))`)
			} else if slices.Contains(columns, "session_id") {
				clauses = append(clauses, `session_id IN (SELECT value FROM json_each(?))`)
			}
		case "agy":
			if table == "conversation_summaries" {
				clauses = append(clauses, `conversation_id IN (SELECT value FROM json_each(?))`)
			}
		case "codex":
			if table == "threads" {
				clauses = append(clauses, `id IN (SELECT value FROM json_each(?))`)
			} else if table == "local_thread_catalog" {
				filter := `thread_id IN (SELECT value FROM json_each(?))`
				if len(tables["local_thread_catalog_hosts"]) > 0 {
					filter += ` AND host_id IN (SELECT host_id FROM local_thread_catalog_hosts WHERE host_kind='local')`
				}
				clauses = append(clauses, filter)
			} else if slices.Contains(columns, "thread_id") {
				clauses = append(clauses, `thread_id IN (SELECT value FROM json_each(?))`)
			}
		}
		for _, clause := range clauses {
			if _, err := collect(table, clause, string(encoded)); err != nil {
				return nil, err
			}
		}
	}
	// Follow incoming foreign keys so message parts and other dependent rows travel together.
	for {
		added := false
		for table := range tables {
			rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_list("+quoteSQL(table)+")")
			if err != nil {
				return nil, err
			}
			type link struct {
				id               int
				parent, from, to string
			}
			var links []link
			counts := map[int]int{}
			for rows.Next() {
				var l link
				var seq int
				var update, del, match string
				if err := rows.Scan(&l.id, &seq, &l.parent, &l.from, &l.to, &update, &del, &match); err != nil {
					rows.Close()
					return nil, err
				}
				counts[l.id]++
				links = append(links, l)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			rows.Close()
			for _, l := range links {
				parent := backups[l.parent]
				if parent == nil {
					continue
				}
				if counts[l.id] != 1 {
					return nil, errors.New("关联表使用复合外键，需要适配后再删除")
				}
				to := l.to
				if to == "" && len(parent.Primary) == 1 {
					to = parent.Primary[0]
				}
				index := slices.Index(parent.Columns, to)
				if index < 0 {
					return nil, errors.New("关联表的外键无效")
				}
				var values []any
				for _, row := range parent.Rows {
					values = append(values, row[index].value())
				}
				raw, _ := json.Marshal(values)
				found, err := collect(table, quoteSQL(l.from)+" IN (SELECT value FROM json_each(?))", string(raw))
				if err != nil {
					return nil, err
				}
				added = added || found
			}
		}
		if !added {
			break
		}
	}
	for _, backup := range backups {
		idColumn := ""
		if provider == "opencode" && (backup.Name == "session" || backup.Name == "session_v2") || provider == "codex" && backup.Name == "threads" {
			idColumn = "id"
		} else if provider == "agy" && backup.Name == "conversation_summaries" {
			idColumn = "conversation_id"
		}
		if idColumn != "" {
			index := slices.Index(backup.Columns, idColumn)
			for _, row := range backup.Rows {
				if index < 0 || !slices.Contains(ids, row[index].Text) {
					return nil, errors.New("关联子会话已变化，请刷新后重新确认删除")
				}
			}
		}
		change.backup.Tables = append(change.backup.Tables, *backup)
	}
	slices.SortFunc(change.backup.Tables, func(a, b TableBackup) int { return strings.Compare(a.Name, b.Name) })
	ok = true
	return change, nil
}

func primaryWhere(table TableBackup, row []SQLValue) (string, []any, error) {
	var clauses []string
	var args []any
	for _, name := range table.Primary {
		index := slices.Index(table.Columns, name)
		if index < 0 || index >= len(row) {
			return "", nil, errors.New("回收站记录的主键无效")
		}
		clauses = append(clauses, quoteSQL(name)+" IS ?")
		args = append(args, row[index].value())
	}
	if len(clauses) == 0 {
		return "", nil, errors.New("回收站记录缺少主键")
	}
	return strings.Join(clauses, " AND "), args, nil
}

func (c *databaseChange) deleteRows(ctx context.Context) error {
	for _, table := range c.backup.Tables {
		for _, row := range table.Rows {
			where, args, err := primaryWhere(table, row)
			if err != nil {
				return err
			}
			if _, err := c.tx.ExecContext(ctx, "DELETE FROM "+quoteSQL(table.Name)+" WHERE "+where, args...); err != nil {
				return err
			}
		}
	}
	return bumpCatalogRevision(ctx, c.tx, c.backup.Tables)
}

func bumpCatalogRevision(ctx context.Context, tx *sql.Tx, backups []TableBackup) error {
	changed := slices.ContainsFunc(backups, func(table TableBackup) bool {
		return table.Name == "local_thread_catalog" && len(table.Rows) > 0
	})
	if !changed {
		return nil
	}
	tables, err := databaseTables(ctx, tx)
	if err != nil {
		return err
	}
	if !slices.Contains(tables["local_thread_catalog_metadata"], "catalog_revision") {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE local_thread_catalog_metadata SET catalog_revision = catalog_revision + 1 WHERE id = 1`)
	return err
}

func restoreDatabase(ctx context.Context, backup DatabaseBackup) (*databaseChange, error) {
	db, err := openDatabase(ctx, backup.Path, true)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, err
	}
	change := &databaseChange{db: db, tx: tx, backup: backup}
	ok := false
	defer func() {
		if !ok {
			tx.Rollback()
			db.Close()
		}
	}()
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys=ON"); err != nil {
		return nil, err
	}
	for _, table := range backup.Tables {
		var columns, marks []string
		for _, column := range table.Columns {
			columns = append(columns, quoteSQL(column))
			marks = append(marks, "?")
		}
		for _, row := range table.Rows {
			where, args, err := primaryWhere(table, row)
			if err != nil {
				return nil, err
			}
			_, existing, err := readSQLRows(ctx, tx, table.Name, where, args...)
			if err != nil {
				return nil, err
			}
			if len(existing) > 0 {
				a, _ := json.Marshal(existing[0])
				b, _ := json.Marshal(row)
				if string(a) != string(b) {
					return nil, fmt.Errorf("%s 已有同主键记录，原数据保留", table.Name)
				}
				continue
			}
			values := make([]any, len(row))
			for i, value := range row {
				values[i] = value.value()
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+quoteSQL(table.Name)+" ("+strings.Join(columns, ",")+") VALUES ("+strings.Join(marks, ",")+")", values...); err != nil {
				return nil, err
			}
		}
	}
	if err := bumpCatalogRevision(ctx, tx, backup.Tables); err != nil {
		return nil, err
	}
	ok = true
	return change, nil
}

func closeChanges(changes []*databaseChange) {
	for _, change := range changes {
		if !change.committed {
			change.tx.Rollback()
		}
		change.db.Close()
	}
}

func commitChanges(changes []*databaseChange) error {
	for _, change := range changes {
		if err := change.tx.Commit(); err != nil {
			return err
		}
		change.committed = true
	}
	return nil
}
