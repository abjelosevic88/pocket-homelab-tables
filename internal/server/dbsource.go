package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- a table from one of your databases (Pocket Database) ----
//
// Pocket Database knows your Postgres, MySQL and SQLite databases and lets
// another app read them — never change them — through a Core link. A table
// here can be made from a table there or from a read-only query, and later
// brings in the latest: rows are matched by the primary key (or, without
// one, by their content), changed ones are updated, new ones added. Rows
// gone there stay here.

const maxDBRows = 5000

func (s *Server) dbConn(ctx context.Context, uid int64, scope string) (*siblingConn, error) {
	sib, err := s.clients(ctx, uid).conn(ctx, appDatabase, scope)
	if err != nil {
		return nil, &userErr{err.Error()}
	}
	return sib, nil
}

// handleDBConnections lists the person's database connections (not Redis).
func (s *Server) handleDBConnections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, err := s.dbConn(ctx, actorID(ctx), scopeDatabasesRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"connections": []any{}, "off": err.Error()})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/connections", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"connections": []any{}, "off": plainErr(appDatabase, err)})
		return
	}
	out := []map[string]any{}
	for _, c := range rows(v, "connections") {
		if str(c, "engine") == "redis" {
			continue
		}
		out = append(out, map[string]any{"id": num(c, "id"), "name": str(c, "name"), "engine": str(c, "engine"), "engine_name": str(c, "engine_name"),
			"database": str(c, "database"), "group": str(c, "group")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

// handleDBObjects lists the tables and views of a connection (one database of it).
func (s *Server) handleDBObjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, err := s.dbConn(ctx, actorID(ctx), scopeDatabasesRead)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	conn := r.URL.Query().Get("conn")
	q := url.Values{}
	if db := r.URL.Query().Get("db"); db != "" {
		q.Set("db", db)
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/connections/"+url.PathEscape(conn)+"/schemas", q)
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appDatabase, err))
		return
	}
	out := []map[string]any{}
	for _, sc := range rows(v) {
		for _, o := range rows(sc["objects"]) {
			out = append(out, map[string]any{"schema": str(o, "schema"), "name": str(o, "name"), "kind": str(o, "kind"), "rows": num(o, "rows")})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"objects": out})
}

// dbGrid reads a table or a query's answer: the column names, the rows as
// text, and the key columns (the primary key; empty for a query).
func (s *Server) dbGrid(ctx context.Context, sib *siblingConn, src *store.DBSource) ([]string, [][]string, []string, bool, error) {
	var cols []string
	var data [][]any
	var key []string
	truncated := false
	base := "/api/v1/connections/" + fmt.Sprint(src.Conn)
	if src.SQL != "" {
		res, err := postJSONTo(ctx, sib, base+"/query", map[string]any{"db": src.DB, "sql": src.SQL})
		if err != nil {
			return nil, nil, nil, false, &userErr{plainErr(appDatabase, err)}
		}
		var last map[string]any
		for _, r := range rows(res["results"]) {
			if e := str(r, "error"); e != "" {
				return nil, nil, nil, false, &userErr{"the query: " + e}
			}
			if r["has_rows"] == true {
				last = r
			}
		}
		if last == nil {
			return nil, nil, nil, false, &userErr{"the query gives no rows (only a SELECT can make a table)"}
		}
		cols, data, truncated = stringsOf(last["columns"]), anyRows(last["rows"]), last["truncated"] == true
	} else {
		q := url.Values{"name": {src.Table}, "limit": {fmt.Sprint(maxDBRows)}}
		if src.DB != "" {
			q.Set("db", src.DB)
		}
		if src.Schema != "" {
			q.Set("schema", src.Schema)
		}
		if t, err := getJSONFrom(ctx, sib, base+"/table", q); err == nil {
			if m, ok := t.(map[string]any); ok {
				key = stringsOf(m["primary_key"])
				if m["rowid"] == true {
					key = []string{"__rowid"}
				}
			}
		}
		v, err := getJSONFrom(ctx, sib, base+"/rows", q)
		if err != nil {
			return nil, nil, nil, false, &userErr{plainErr(appDatabase, err)}
		}
		m, _ := v.(map[string]any)
		cols, data = stringsOf(m["columns"]), anyRows(m["rows"])
		truncated = num(m, "total") > float64(len(data))
	}
	grid := make([][]string, 0, len(data))
	for _, r := range data {
		row := make([]string, len(cols))
		for i := range cols {
			if i < len(r) && r[i] != nil {
				row[i] = tables.ToText(r[i])
			}
		}
		grid = append(grid, row)
	}
	return cols, grid, key, truncated, nil
}

func stringsOf(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		out = append(out, tables.ToText(x))
	}
	return out
}

func anyRows(v any) [][]any {
	arr, _ := v.([]any)
	out := make([][]any, 0, len(arr))
	for _, x := range arr {
		if r, ok := x.([]any); ok {
			out = append(out, r)
		}
	}
	return out
}

// rowRef is how a database row is known again: by its key, else by its content.
func rowRef(cols []string, row []string, key []string) string {
	var parts []string
	if len(key) > 0 {
		for _, k := range key {
			for i, c := range cols {
				if c == k && i < len(row) {
					parts = append(parts, row[i])
				}
			}
		}
		if len(parts) == len(key) {
			return "db:" + strings.Join(parts, "\x1f")
		}
	}
	h := sha1.Sum([]byte(strings.Join(row, "\x1f")))
	return "db#" + hex.EncodeToString(h[:10])
}

type dbIn struct {
	Conn     int64  `json:"conn"`
	ConnName string `json:"conn_name"`
	DB       string `json:"db"`
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	SQL      string `json:"sql"`
	Name     string `json:"name"`
	BaseID   int64  `json:"base_id"`
	TableID  int64  `json:"table_id"` // bring in the latest into a table made this way
}

// handleFromDB makes a table from a database table or query, or (table_id)
// brings in the latest rows of the one a table was made from.
func (s *Server) handleFromDB(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in dbIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.TableID > 0 {
		n, upd, err := s.refreshDB(ctx, uid, in.TableID)
		if err != nil {
			s.userFail(w, err, "bring in")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"made": n, "updated": upd})
		return
	}
	src := &store.DBSource{Conn: in.Conn, ConnName: clip(in.ConnName, 200), DB: in.DB, Schema: in.Schema, Table: in.Table, SQL: strings.TrimSpace(in.SQL)}
	if src.Conn <= 0 || (src.Table == "" && src.SQL == "") {
		writeErr(w, http.StatusBadRequest, "pick a database table, or write a query")
		return
	}
	sib, err := s.dbConn(ctx, uid, scopeDatabasesQuery)
	if err != nil {
		s.userFail(w, err, "database")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cols, grid, key, truncated, err := s.dbGrid(cctx, sib, src)
	if err != nil {
		s.userFail(w, err, "database")
		return
	}
	if len(cols) == 0 {
		writeErr(w, http.StatusBadRequest, "there are no columns")
		return
	}
	src.Key = key
	// the key column of a SQLite table without a primary key is only for matching
	keep := make([]int, 0, len(cols))
	for i, c := range cols {
		if c != "__rowid" {
			keep = append(keep, i)
		}
	}
	head := make([]string, 0, len(keep))
	for _, i := range keep {
		head = append(head, cols[i])
	}
	full := [][]string{head}
	for _, r := range grid {
		row := make([]string, 0, len(keep))
		for _, i := range keep {
			row = append(row, r[i])
		}
		full = append(full, row)
	}
	name := firstNonEmpty(strings.TrimSpace(in.Name), src.Table, "Query from "+src.ConnName)
	spec := specFromSheet(name, full)
	spec.Rows = nil
	spec.Description = "From " + firstNonEmpty(src.ConnName, "a database") + " (Pocket Database), " + today() + "."
	b, err := s.build(ctx, uid, specBase{Name: firstNonEmpty(src.ConnName, "Database"), Icon: "🗄️", Tables: []specTable{spec}}, in.BaseID, false)
	if err != nil {
		s.userFail(w, err, "make table")
		return
	}
	var t *store.Sheet
	for _, x := range b.Tables {
		if t == nil || x.ID > t.ID {
			t = x
		}
	}
	t.Settings.DB = src
	if err := s.store.UpdateSheet(ctx, t); err != nil {
		s.fail(w, err, "save")
		return
	}
	made, _, err := s.fillFromDB(ctx, uid, t.ID, cols, grid, key)
	if err != nil {
		s.userFail(w, err, "rows")
		return
	}
	t, _ = s.store.GetSheet(ctx, uid, t.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"table": t, "made": made, "truncated": truncated})
}

// fillFromDB adds the rows not here yet and updates the ones that changed.
func (s *Server) fillFromDB(ctx context.Context, uid, sheetID int64, cols []string, grid [][]string, key []string) (int, int, error) {
	ld, err := s.loader(uid).load(ctx, sheetID, false)
	if err != nil {
		return 0, 0, err
	}
	made, updated := 0, 0
	for _, row := range grid {
		vals := map[string]any{}
		for i, c := range cols {
			if c == "__rowid" || i >= len(row) {
				continue
			}
			if f := ld.field(c); f != nil {
				vals[tables.Key(f.ID)] = row[i]
				if row[i] == "" {
					vals[tables.Key(f.ID)] = nil
				}
			}
		}
		ref := rowRef(cols, row, key)
		if old, err := s.store.RecordBySource(ctx, uid, sheetID, ref); err == nil {
			if old.DeletedAt != "" {
				continue // deleted here on purpose: not brought back
			}
			changes, _, err := s.setValues(ctx, uid, ld, old, vals, true)
			if err != nil || len(changes) == 0 {
				continue
			}
			if s.store.SaveRecord(ctx, old) == nil {
				updated++
				_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: old.ID, Kind: "change", Data: map[string]any{"changes": changes}, App: appDatabase.slug})
			}
			continue
		}
		if _, err := s.addRecord(ctx, uid, ld, recordIn{Values: vals, Ref: ref, Lenient: true}, appDatabase.slug); err == nil {
			made++
		}
	}
	if c := ld.sheet.Settings.Calendar; c != nil && c.On && made+updated > 0 {
		go s.syncSheetCalendar(context.WithoutCancel(ctx), uid, sheetID)
	}
	return made, updated, nil
}

// refreshDB brings in the latest rows for a table made from a database.
func (s *Server) refreshDB(ctx context.Context, uid, sheetID int64) (int, int, error) {
	t, err := s.store.GetSheet(ctx, uid, sheetID)
	if err != nil {
		return 0, 0, err
	}
	src := t.Settings.DB
	if src == nil {
		return 0, 0, &userErr{"this table was not made from a database"}
	}
	sib, err := s.dbConn(ctx, uid, scopeDatabasesQuery)
	if err != nil {
		return 0, 0, err
	}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cols, grid, key, _, err := s.dbGrid(cctx, sib, src)
	if err != nil {
		return 0, 0, err
	}
	if len(src.Key) > 0 {
		key = src.Key // match the way the table was made
	}
	made, upd, err := s.fillFromDB(ctx, uid, sheetID, cols, grid, key)
	if err == nil {
		src.At = store.Now()
		t.Settings.DB = src
		_ = s.store.UpdateSheet(ctx, t)
	}
	return made, upd, err
}
