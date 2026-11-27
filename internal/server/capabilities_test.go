package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/config"
)

// fakeCapCore is a Pocket Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|capture|read|write> from Pocket Hub
// and remembers the manifest Tables registers with.
type fakeCapCore struct {
	*httptest.Server
	mu       sync.Mutex
	manifest map[string]any
}

func newCapApp(t *testing.T) (*app, *fakeCapCore) {
	t.Helper()
	fc := &fakeCapCore{}
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "tables:read", "write": "tables:write"}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			fc.mu.Lock()
			fc.manifest = m
			fc.mu.Unlock()
			io.WriteString(w, `{}`)
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(in.Token, "_")
			if len(parts) != 3 || parts[0] != "pcx" || scopes[parts[2]] == "" {
				io.WriteString(w, `{"active":false}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "tables", "scope": scopes[parts[2]],
				"user": map[string]any{"id": 1, "sub": parts[1] + "@example.com", "email": parts[1] + "@example.com", "name": parts[1], "role": "member"},
				"app":  map[string]any{"id": 9, "slug": "hub", "name": "Pocket Hub"}})
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(fc.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL, c.CoreRegistrationKey = fc.URL, "key" }), fc
}

// items are the rows of a capability answer.
func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

// item finds the row with that title.
func item(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range items(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

// titles are the answer's titles, in order.
func titles(t *testing.T, m map[string]any) string {
	t.Helper()
	var out []string
	for _, it := range items(t, m) {
		out = append(out, it["title"].(string))
	}
	return strings.Join(out, ", ")
}

// library gives a person a base "Library" with the tables Books (two rows)
// and Book clubs (none), and returns their ids and the rows' ids.
func (a *app) library(auth string) map[string]string {
	a.t.Helper()
	b := a.ok(auth, "POST", "/api/v1/bases", map[string]any{"name": "Library", "empty": true})
	books := id(a.ok(auth, "POST", "/api/v1/bases/"+id(b)+"/tables", map[string]any{"name": "Books"}))
	clubs := id(a.ok(auth, "POST", "/api/v1/bases/"+id(b)+"/tables", map[string]any{"name": "Book clubs"}))
	for _, f := range []map[string]any{{"name": "Author", "type": "text"}, {"name": "Pages", "type": "number"}, {"name": "Scan", "type": "attachment"},
		{"name": "Added", "type": "created"}, {"name": "Read on", "type": "date"}, {"name": "Shelf", "type": "text"}} {
		a.ok(auth, "POST", "/api/v1/tables/"+books+"/fields", f)
	}
	scan := a.upload(auth, "dune-cover.jpg", []byte("\xff\xd8\xff fake"))
	dune := a.ok(auth, "POST", "/api/v1/tables/"+books+"/records", map[string]any{"values": map[string]any{"Name": "Dune", "Author": "Frank Herbert", "Pages": 612,
		"Status": "Done", "Shelf": "Hall", "Scan": []any{map[string]any{"id": scan, "name": "dune-cover.jpg"}},
		"Notes": "A desert planet, a boy who sees too far ahead, and the worms that make the spice everybody is fighting over.\nRead it twice."}})
	emma := a.ok(auth, "POST", "/api/v1/tables/"+books+"/records", map[string]any{"values": map[string]any{"Name": "Emma", "Author": "Jane Austen", "Pages": 474}})
	return map[string]string{"base": id(b), "books": books, "clubs": clubs, "dune": id(dune), "emma": id(emma)}
}

// stamp sets when a row was added and last changed.
func (a *app) stamp(rowID string, created, updated time.Time) {
	a.t.Helper()
	const layout = "2006-01-02T15:04:05.000Z"
	if _, err := a.st.DB.Exec(`UPDATE records SET created_at = ?, updated_at = ? WHERE id = ?`, created.UTC().Format(layout), updated.UTC().Format(layout), n(rowID)); err != nil {
		a.t.Fatal(err)
	}
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	ids := a.library(a.alice)
	// tables by name first, then the row named by the words, then the rows that have them elsewhere
	diary := a.ok(a.alice, "POST", "/api/v1/tables/"+ids["clubs"]+"/records", map[string]any{"values": map[string]any{"Name": "Notebook of the club"}})
	a.ok(a.alice, "POST", "/api/v1/tables/"+ids["clubs"]+"/records", map[string]any{"values": map[string]any{"Name": "Thursdays", "Notes": "bring a book"}})
	res := a.ok(a.alice, "GET", "/api/v1/pocket/search?q=book", nil)
	if got := titles(t, res); got != "Books, Book clubs, Notebook of the club, Thursdays" {
		t.Fatalf("book: %s", got)
	}
	books := item(t, res, "Books")
	if books["kind"] != "table" || books["id"] != "table:"+ids["books"] || books["url"] != "/t/"+ids["books"] || books["snippet"] != "In Library · 2 rows" || books["date"] == nil {
		t.Fatalf("the table: %v", books)
	}
	if it := item(t, res, "Notebook of the club"); it["kind"] != "row" || it["id"] != "row:"+id(diary) || it["url"] != "/r/"+id(diary) || it["snippet"] != "Book clubs in Library" {
		t.Fatalf("a row with nothing but a name: %v", it)
	}
	if it := item(t, res, "Thursdays"); it["snippet"] != "Book clubs in Library · Notes: bring a book" {
		t.Fatalf("found in the notes: %v", it)
	}
	// a row's line: its table, then its first values; empty ones, files and the row's own times are left out
	dune := item(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=dune", nil), "Dune")
	line, _ := dune["snippet"].(string)
	if !strings.HasPrefix(line, "Books in Library · Notes: A desert planet, a boy who sees too far ahead") || !strings.HasSuffix(line, "… · Status: Done · Author: Frank Herbert") ||
		strings.Contains(line, "\n") || strings.Contains(line, "jpg") || strings.Contains(line, "Added") || dune["date"] == nil {
		t.Fatalf("Dune: %v", dune)
	}
	// the field the words were found in comes first, whichever it is
	if it := item(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=hall", nil), "Dune"); !strings.HasPrefix(it["snippet"].(string), "Books in Library · Shelf: Hall · Notes: ") ||
		!strings.HasSuffix(it["snippet"].(string), " · Status: Done") {
		t.Fatalf("found on the shelf: %v", it)
	}
	if it := item(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=austen", nil), "Emma"); it["snippet"] != "Books in Library · Author: Jane Austen · Pages: 474" {
		t.Fatalf("Emma: %v", it)
	}
	// a row without a name is still a row
	blank := a.ok(a.alice, "POST", "/api/v1/tables/"+ids["books"]+"/records", map[string]any{"values": map[string]any{"Author": "Anonymous"}})
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=anonymous", nil)); len(got) != 1 || got[0]["title"] != "Row "+itoa(int64(num(blank, "number"))) {
		t.Fatalf("no name: %v", got)
	}
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=book&limit=1", nil)); len(got) != 1 || got[0]["kind"] != "table" {
		t.Fatalf("limit 1: %v", got)
	}
	for _, q := range []string{"", "??", "*", "%20", "-"} {
		if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q="+q, nil)); len(got) != 0 {
			t.Fatalf("%q finds %v", q, got)
		}
	}
	// a row in the bin is not found
	a.expect(204, a.alice, "DELETE", "/api/v1/records/"+ids["emma"], nil)
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=emma", nil)); len(got) != 0 {
		t.Fatalf("a deleted row: %v", got)
	}
	// another person's tables and rows never appear
	for _, q := range []string{"book", "dune", "herbert", "library"} {
		if got := items(t, a.ok(a.bob, "GET", "/api/v1/pocket/search?q="+q, nil)); len(got) != 0 {
			t.Fatalf("bob finds alice's %q: %v", q, got)
		}
	}
	a.expect(401, "", "GET", "/api/v1/pocket/search?q=dune", nil)
	// the old route still answers as before
	old := a.ok(a.alice, "GET", "/api/v1/search?q=book", nil)
	if len(old["tables"].([]any)) != 2 || len(old["items"].([]any)) != 2 || old["query"] != "book" {
		t.Fatalf("/search changed: %v", old)
	}
	if h := old["items"].([]any)[1].(map[string]any); h["title"] != "Thursdays" || h["field"] != "Notes" || h["snippet"] != "bring a book" || h["table"] != "Book clubs" ||
		h["base"] != "Library" || h["url"] != "http://tables.test/r/"+id(h) {
		t.Fatalf("/search row: %v", h)
	}
	if tb := old["tables"].([]any)[0].(map[string]any); tb["name"] != "Books" || tb["base"] != "Library" || num(tb, "rows") != 2 || tb["url"] != "http://tables.test/t/"+ids["books"] {
		t.Fatalf("/search table: %v", tb)
	}
}

func TestPocketDay(t *testing.T) {
	a := newApp(t, nil)
	ids := a.library(a.alice)
	now := time.Now()
	today := now.Format("2006-01-02")
	before := now.AddDate(0, 0, -3)
	// Dune: added and read today (comes once, as added). Emma: added before, changed today.
	a.ok(a.alice, "PUT", "/api/v1/records/"+ids["dune"], map[string]any{"values": map[string]any{"Read on": today}})
	a.stamp(ids["emma"], before, now)
	// in another table: one added before with a time today, one with nothing to do with today
	trips, _ := a.newTable(a.alice, "Travel")
	a.ok(a.alice, "POST", "/api/v1/tables/"+trips+"/fields", map[string]any{"name": "Leave", "type": "date", "options": map[string]any{"time": true}})
	rome := a.ok(a.alice, "POST", "/api/v1/tables/"+trips+"/records", map[string]any{"values": map[string]any{"Name": "Rome", "Leave": today + "T14:30"}})
	oslo := a.ok(a.alice, "POST", "/api/v1/tables/"+trips+"/records", map[string]any{"values": map[string]any{"Name": "Oslo", "Leave": now.AddDate(0, 0, 9).Format("2006-01-02") + "T08:00"}})
	a.stamp(id(rome), before, before)
	a.stamp(id(oslo), before, before)

	day := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+today, nil)
	if day["date"] != today || day["summary"] != "1 row added in Books; 1 row changed in Books; 1 row with this date in Table 1" || len(items(t, day)) != 3 {
		t.Fatalf("day: %v", day)
	}
	if it := item(t, day, "Dune"); it["kind"] != "added" || it["id"] != "row:"+ids["dune"] || it["sub"] != "Books" || it["url"] != "/r/"+ids["dune"] || len(it["time"].(string)) != 5 {
		t.Fatalf("added: %v", it)
	}
	if it := item(t, day, "Emma"); it["kind"] != "changed" || it["sub"] != "Books" || it["time"] != now.Format("15:04") {
		t.Fatalf("changed: %v", it)
	}
	if it := item(t, day, "Rome"); it["kind"] != "dated" || it["sub"] != "Table 1 · Leave" || it["time"] != "14:30" || it["url"] != "/r/"+id(rome) {
		t.Fatalf("dated: %v", it)
	}
	// the rows about the day lead, so a day of many new rows keeps them in sight
	if got := titles(t, day); !strings.HasPrefix(got, "Rome, ") {
		t.Fatalf("order: %s", got)
	}
	// more of them, in more tables
	a.ok(a.alice, "POST", "/api/v1/tables/"+trips+"/records", map[string]any{"values": map[string]any{"Name": "Lisbon"}})
	a.ok(a.alice, "POST", "/api/v1/tables/"+ids["clubs"]+"/records", map[string]any{"values": map[string]any{"Name": "Thursdays"}})
	if day := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+today, nil); day["summary"] != "3 rows added in Books, Book clubs and Table 1; 1 row changed in Books; 1 row with this date in Table 1" {
		t.Fatalf("a fuller day: %v", day["summary"])
	}
	// the day the older rows were added, and the day Oslo leaves
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+before.Format("2006-01-02"), nil); d["summary"] != "3 rows added in Books and Table 1" || len(items(t, d)) != 3 {
		t.Fatalf("three days ago: %v", d)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+now.AddDate(0, 0, 9).Format("2006-01-02"), nil); d["summary"] != "1 row with this date in Table 1" || item(t, d, "Oslo")["time"] != "08:00" {
		t.Fatalf("a day to come: %v", d)
	}
	// the day is the asker's: half past eleven at night in London is the next day in Sarajevo
	late := time.Date(2024, 3, 10, 23, 30, 0, 0, time.UTC)
	a.stamp(id(oslo), late, late)
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2024-03-11?tz=Europe/Sarajevo", nil); d["summary"] != "1 row added in Table 1" || item(t, d, "Oslo")["time"] != "00:30" {
		t.Fatalf("Sarajevo: %v", d)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2024-03-10?tz=Europe/London", nil); item(t, d, "Oslo")["time"] != "23:30" {
		t.Fatalf("London: %v", d)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2024-03-10?tz=Europe/Sarajevo", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("the day before in Sarajevo: %v", d)
	}
	// a row in the bin is gone from the day; somebody else's day is empty
	a.expect(204, a.alice, "DELETE", "/api/v1/records/"+ids["emma"], nil)
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+today, nil); strings.Contains(d["summary"].(string), "changed") {
		t.Fatalf("a deleted row: %v", d)
	}
	if d := a.ok(a.bob, "GET", "/api/v1/pocket/day/"+today, nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("bob sees alice's day: %v", d)
	}
	a.expect(400, a.alice, "GET", "/api/v1/pocket/day/yesterday", nil)
	a.expect(401, "", "GET", "/api/v1/pocket/day/"+today, nil)
	// the old route still answers as before: the rows with a date on the day, added today or not
	old := a.ok(a.alice, "GET", "/api/v1/day/"+today, nil)["items"].([]any)
	if len(old) != 2 || old[0].(map[string]any)["title"] != "Dune" || old[0].(map[string]any)["field"] != "Read on" || old[1].(map[string]any)["date"] != today+"T14:30" {
		t.Fatalf("/day changed: %v", old)
	}
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	// no tables: nothing to show
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || w["metrics"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("without tables: %v", w)
	}
	ids := a.library(a.alice)
	old, _ := a.newTable(a.alice, "Archive")
	now := time.Now()
	long := now.AddDate(0, 0, -40)
	for _, name := range []string{"2019", "2020"} {
		a.stamp(id(a.ok(a.alice, "POST", "/api/v1/tables/"+old+"/records", map[string]any{"values": map[string]any{"Name": name}})), long, long)
	}
	aged := func(tableID string, at time.Time) {
		t.Helper()
		if _, err := a.st.DB.Exec(`UPDATE sheets SET updated_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339Nano), n(tableID)); err != nil {
			t.Fatal(err)
		}
	}
	aged(old, long)
	aged(ids["clubs"], long.AddDate(0, 0, -1))
	// Emma changed three days ago, Dune today; a row in the bin is not counted
	a.stamp(ids["emma"], long, now.AddDate(0, 0, -3))
	gone := a.ok(a.alice, "POST", "/api/v1/tables/"+ids["books"]+"/records", map[string]any{"values": map[string]any{"Name": "Lost"}})
	a.expect(204, a.alice, "DELETE", "/api/v1/records/"+id(gone), nil)

	w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	rows := items(t, w)
	if w["line"] != "1 row added or changed today" || w["url"] != "/" || w["metrics"] != nil || len(rows) != 3 {
		t.Fatalf("widget: %v", w)
	}
	for i, want := range [][3]string{{"Books", "2 rows added or changed in the last week", ids["books"]}, {"Table 1", "2 rows", old}, {"Book clubs", "No rows yet", ids["clubs"]}} {
		if r := rows[i]; r["title"] != want[0] || r["detail"] != want[1] || r["id"] != "table:"+want[2] || r["url"] != "/t/"+want[2] {
			t.Fatalf("row %d: %v, want %v", i, r, want)
		}
	}
	// a change in a table that slept brings it to the top
	a.ok(a.alice, "PUT", "/api/v1/records/"+id(a.ok(a.alice, "GET", "/api/v1/tables/"+old+"/records", nil)["records"].([]any)[0].(map[string]any)),
		map[string]any{"values": map[string]any{"Notes": "found the box"}})
	w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	if rows := items(t, w); w["line"] != "2 rows added or changed today" || rows[0]["title"] != "Table 1" || rows[0]["detail"] != "1 row added or changed in the last week" {
		t.Fatalf("after a change: %v", w)
	}
	// nothing lately: no line, the tables with their size
	for _, rid := range []string{ids["dune"], ids["emma"]} {
		a.stamp(rid, long, long)
	}
	for _, r := range a.ok(a.alice, "GET", "/api/v1/tables/"+old+"/records", nil)["records"].([]any) {
		a.stamp(id(r.(map[string]any)), long, long)
	}
	if w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || items(t, w)[0]["detail"] != "2 rows" {
		t.Fatalf("a quiet week: %v", w)
	}
	// only the newest few tables
	for range 5 {
		a.ok(a.alice, "POST", "/api/v1/bases/"+ids["base"]+"/tables", map[string]any{"name": "More"})
	}
	if rows := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)); len(rows) != 5 || rows[0]["title"] != "More" || rows[4]["title"] != "More" {
		t.Fatalf("many tables: %v", rows)
	}
	// bob still has nothing of alice's
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("bob's widget: %v", w)
	}
	a.expect(401, "", "GET", "/api/v1/pocket/widget", nil)
}

// The widget counts the week's rows from their own index, not from every row a person keeps.
func TestPocketWidgetReadsFewRows(t *testing.T) {
	a := newApp(t, nil)
	rows, err := a.st.DB.Query(`EXPLAIN QUERY PLAN SELECT sheet_id, COUNT(1), COALESCE(SUM(updated_at >= ?), 0), MAX(updated_at) FROM records
		WHERE user_id = ? AND deleted_at IS NULL AND updated_at >= ? GROUP BY sheet_id`, "2026-01-02T00:00:00", 1, "2026-01-01T00:00:00")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if !strings.Contains(plan, "COVERING INDEX records_changed (user_id=? AND updated_at>?)") {
		t.Fatalf("the week's rows are not read from their index:\n%s", plan)
	}
}

// A link token with one pocket:* scope reaches its one route and nothing else.
func TestPocketLinkTokens(t *testing.T) {
	a, _ := newCapApp(t)
	ids := a.library(a.alice)
	today := time.Now().Format("2006-01-02")
	line := map[string]any{"request_id": "req-0000000000000001", "text": "Dune"}
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_alice_search", "GET", "/api/v1/pocket/search?q=dune", nil, 200},
		{"pcx_alice_day", "GET", "/api/v1/pocket/day/" + today, nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_alice_search", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/search?q=dune", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/search?q=dune", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		// nothing can be kept here: there is no such route, and the scope opens nothing else
		{"pcx_alice_capture", "POST", "/api/v1/pocket/capture", line, 404},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_capture", "POST", "/api/v1/tables/" + ids["books"] + "/records", map[string]any{"values": map[string]any{"Name": "x"}}, 403},
		// the rest of the app
		{"pcx_alice_search", "GET", "/api/v1/search?q=dune", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/bases", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/tables/" + ids["books"] + "/records", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/records/" + ids["dune"], nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/day/" + today, nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/today", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/summary", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/tables/" + ids["books"], nil, 403},
		{"pcx_alice_widget", "PUT", "/api/v1/records/" + ids["dune"], map[string]any{"values": map[string]any{"Name": "x"}}, 403},
		{"pcx_alice_widget", "DELETE", "/api/v1/records/" + ids["dune"], nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/pocket/search?q=dune", nil, 405},
		{"pcx_alice_day", "GET", "/api/v1/settings", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/export.json", nil, 403},
		// the app's own scopes do not open the new routes
		{"pcx_alice_read", "GET", "/api/v1/pocket/search?q=dune", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "POST", "/api/v1/pocket/capture", line, 404},
		{"pcx_alice_read", "GET", "/api/v1/search?q=dune", nil, 200},
		{"pcx_alice_read", "GET", "/api/v1/day/" + today, nil, 200},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, m, _ := a.call(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%v)", c.tok, c.method, c.path, code, c.want, m)
		}
	}
	a.expect(404, a.alice, "POST", "/api/v1/pocket/capture", line)
	// the token acts for its person only
	if got := items(t, a.ok("pcx_alice_search", "GET", "/api/v1/pocket/search?q=dune", nil)); len(got) != 1 || got[0]["title"] != "Dune" {
		t.Fatalf("alice through the link: %v", got)
	}
	if got := items(t, a.ok("pcx_bob_search", "GET", "/api/v1/pocket/search?q=dune", nil)); len(got) != 0 {
		t.Fatalf("bob through the link finds alice's row: %v", got)
	}
	if w := a.ok("pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil); len(items(t, w)) != 2 || w["line"] != "2 rows added or changed today" {
		t.Fatalf("alice's widget through the link: %v", w)
	}
	if w := a.ok("pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil); len(items(t, w)) != 0 {
		t.Fatalf("bob's widget through the link: %v", w)
	}
	if d := a.ok("pcx_alice_day", "GET", "/api/v1/pocket/day/"+today, nil); d["summary"] != "2 rows added in Books" {
		t.Fatalf("alice's day through the link: %v", d)
	}
	if d := a.ok("pcx_bob_day", "GET", "/api/v1/pocket/day/"+today, nil); len(items(t, d)) != 0 {
		t.Fatalf("bob's day through the link: %v", d)
	}
	// nothing was added or changed on the way
	if l := a.ok(a.alice, "GET", "/api/v1/tables/"+ids["books"]+"/records", nil); num(l, "total") != 2 {
		t.Fatalf("alice's rows: %v", l["total"])
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), false},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/day/2026-01-02"), false},
		{core.ScopeSearch, req("GET", "/api/v1/search"), false},
		{core.ScopeWidget, req("GET", "/api/v1/summary"), false},
		{core.ScopeWidget, req("GET", "/api/v1/bases"), false},
		{core.ScopeCapture, req("POST", "/api/v1/tables/3/records"), false},
		{"tables:read", req("GET", "/api/v1/pocket/search"), false},
		{"tables:read", req("GET", "/api/v1/pocket/day/2026-01-02"), false},
		{"tables:read", req("GET", "/api/v1/pocket/widget"), false},
		{"tables:write", req("POST", "/api/v1/pocket/capture"), false},
		{"tables:read", req("GET", "/api/v1/search"), true},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Tables tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	a, fc := newCapApp(t)
	if err := a.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	fc.mu.Lock()
	m := fc.manifest
	fc.mu.Unlock()
	if m["slug"] != "tables" || m["category"] != "Work" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	if search == nil || day == nil || day["label"] != "Tables" || day["one"] != "row" || day["many"] != "rows" || widget == nil || widget["title"] != "Tables" || m["capture"] != nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, m["capture"])
	}
	if bangs := search["bangs"].([]any); len(bangs) != 2 || bangs[0] != "tables" || bangs[1] != "table" || len(search["kinds"].([]any)) != 2 {
		t.Fatalf("search: %v", search)
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"tables:read", "tables:write", core.ScopeMoveExport, core.ScopeMoveImport, core.ScopeSearch, core.ScopeDay, core.ScopeWidget} {
		if !offered[sc] {
			t.Errorf("%s is not offered: %v", sc, offered)
		}
	}
	if offered[core.ScopeCapture] {
		t.Errorf("a quick line is offered, but there is nowhere to keep one")
	}
}

// A match found in the lowercase text can sit past the end of the value itself.
func TestSnippetStaysInsideTheText(t *testing.T) {
	text := strings.Repeat("Ⱥ", 100) + " needle"
	low := strings.ToLower(text)
	if got := snippet(text, strings.Index(low, "needle"), len("needle")); got == "" {
		t.Fatal("empty snippet")
	}
	if got := snippet("short", 400, 6); got != "short" {
		t.Fatalf("out of range: %q", got)
	}
}
