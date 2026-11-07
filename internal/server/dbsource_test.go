package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A table from a database in Pocket Database: made once, then the latest
// rows brought in (a changed one updated, a new one added, nothing twice).
func TestTableFromDatabase(t *testing.T) {
	var mu sync.Mutex
	data := [][]any{{1, "Ana", "2026-11-01", 12.5}, {2, "Emir", "2026-11-03", 7}}
	db := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/connections/7/table":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "orders", "primary_key": []string{"id"}})
		case r.URL.Path == "/api/v1/connections/7/rows":
			if r.URL.Query().Get("name") != "orders" {
				http.Error(w, `{"error":"no table"}`, 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"columns": []string{"id", "customer", "ordered", "total"}, "rows": data, "total": len(data)})
		case r.URL.Path == "/api/v1/connections/7/query":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"has_rows": true, "columns": []string{"customer", "n"}, "rows": [][]any{{"Ana", 3}}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer db.Close()
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"database_mode": "direct", "database_url": db.URL, "database_token": "pdb_test"})
	made := a.ok(a.alice, "POST", "/api/v1/from-db", map[string]any{"conn": 7, "conn_name": "Shop", "table": "orders"})
	if num(made, "made") != 2 {
		t.Fatalf("made: %v", made)
	}
	tid := id(made["table"].(map[string]any))
	page := a.ok(a.alice, "GET", "/api/v1/tables/"+tid, nil)
	types := map[string]string{}
	for _, f := range page["fields"].([]any) {
		m := f.(map[string]any)
		types[m["name"].(string)] = m["type"].(string)
	}
	if types["ordered"] != "date" || types["total"] != "number" {
		t.Fatalf("types: %v", types)
	}
	mu.Lock()
	data[1][3] = 9.5
	data = append(data, []any{3, "Lejla", "2026-11-05", 20})
	mu.Unlock()
	got := a.ok(a.alice, "POST", "/api/v1/from-db", map[string]any{"table_id": n(tid)})
	if num(got, "made") != 1 || num(got, "updated") != 1 {
		t.Fatalf("refresh: %v", got)
	}
	rows := a.ok(a.alice, "GET", "/api/v1/tables/"+tid+"/records?named=1", nil)["records"].([]any)
	if len(rows) != 3 || !strings.Contains(rows[1].(map[string]any)["named"].(map[string]any)["total"].(string), "9.5") {
		t.Fatalf("rows: %v", rows)
	}
	// a query works too
	q := a.ok(a.alice, "POST", "/api/v1/from-db", map[string]any{"conn": 7, "conn_name": "Shop", "sql": "select customer, count(*) n from orders group by 1", "name": "Per customer"})
	if num(q, "made") != 1 {
		t.Fatalf("query: %v", q)
	}
	// Bob cannot bring Alice's table in
	a.expect(404, a.bob, "POST", "/api/v1/from-db", map[string]any{"table_id": n(tid)})
}
