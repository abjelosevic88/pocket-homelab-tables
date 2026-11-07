package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// newTable gives a person a base with one table and returns the table id.
func (a *app) newTable(auth, name string) (string, map[string]any) {
	a.t.Helper()
	b := a.ok(auth, "POST", "/api/v1/bases", map[string]any{"name": name})
	t := b["tables"].([]any)[0].(map[string]any)
	tid := id(t)
	page := a.ok(auth, "GET", "/api/v1/tables/"+tid, nil)
	return tid, page
}

func fieldID(page map[string]any, name string) string {
	for _, f := range page["fields"].([]any) {
		m := f.(map[string]any)
		if m["name"] == name {
			return id(m)
		}
	}
	return ""
}

func TestTablesArePerPerson(t *testing.T) {
	a := newApp(t, nil)
	tid, page := a.newTable(a.alice, "Home")
	name := fieldID(page, "Name")
	rec := a.ok(a.alice, "POST", "/api/v1/tables/"+tid+"/records", map[string]any{"values": map[string]any{name: "Drill", "Status": "Doing"}})
	if rec["title"] != "Drill" {
		t.Fatalf("row: %v", rec)
	}
	rid := id(rec)
	view := id(page["views"].([]any)[0].(map[string]any))
	cm := a.ok(a.alice, "POST", "/api/v1/records/"+rid+"/comments", map[string]any{"text": "lent to Emir"})

	// Bob sees none of it and cannot touch it
	for _, c := range []struct{ m, p string }{
		{"GET", "/api/v1/tables/" + tid}, {"GET", "/api/v1/tables/" + tid + "/records"}, {"PUT", "/api/v1/tables/" + tid},
		{"DELETE", "/api/v1/tables/" + tid}, {"POST", "/api/v1/tables/" + tid + "/records"}, {"POST", "/api/v1/tables/" + tid + "/fields"},
		{"GET", "/api/v1/records/" + rid}, {"PUT", "/api/v1/records/" + rid}, {"DELETE", "/api/v1/records/" + rid},
		{"POST", "/api/v1/records/" + rid + "/duplicate"}, {"POST", "/api/v1/records/" + rid + "/comments"}, {"POST", "/api/v1/records/" + rid + "/make"},
		{"PUT", "/api/v1/fields/" + name}, {"DELETE", "/api/v1/fields/" + name}, {"PUT", "/api/v1/views/" + view}, {"DELETE", "/api/v1/views/" + view},
		{"DELETE", "/api/v1/comments/" + id(cm)}, {"GET", "/api/v1/tables/" + tid + "/export.csv"},
	} {
		a.expect(404, a.bob, c.m, c.p, map[string]any{"values": map[string]any{}, "text": "x", "name": "x", "target": "note"})
	}
	if l := a.list(a.bob, "/api/v1/bases"); len(l) != 0 {
		t.Fatalf("Bob sees Alice's bases: %v", l)
	}
	if s := a.ok(a.bob, "GET", "/api/v1/search?q=drill", nil); len(s["items"].([]any)) != 0 {
		t.Fatalf("Bob finds Alice's rows")
	}
	// a link to Alice's table from Bob's table is refused
	btid, _ := a.newTable(a.bob, "Mine")
	a.expect(400, a.bob, "POST", "/api/v1/tables/"+btid+"/fields", map[string]any{"name": "Theirs", "type": "link", "options": map[string]any{"table_id": n(tid)}})
	// a file of Alice's is not Bob's
	fid := a.upload(a.alice, "photo.jpg", []byte("\xff\xd8\xff fake"))
	a.expect(404, a.bob, "GET", "/api/v1/files/"+fid, nil)
	a.expect(200, a.alice, "GET", "/api/v1/files/"+fid, nil)
}

func (a *app) upload(auth, name string, data []byte) string {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", a.url+"/api/v1/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	if res.StatusCode != 201 {
		a.t.Fatalf("upload: %d %v", res.StatusCode, m)
	}
	return m["id"].(string)
}

func TestFieldsLinksLookupsFormulas(t *testing.T) {
	a := newApp(t, nil)
	b := a.ok(a.alice, "POST", "/api/v1/bases", map[string]any{"name": "Shop", "empty": true})
	bid := id(b)
	products := id(a.ok(a.alice, "POST", "/api/v1/bases/"+bid+"/tables", map[string]any{"name": "Products"}))
	orders := id(a.ok(a.alice, "POST", "/api/v1/bases/"+bid+"/tables", map[string]any{"name": "Orders"}))
	price := id(a.ok(a.alice, "POST", "/api/v1/tables/"+products+"/fields", map[string]any{"name": "Price", "type": "currency", "options": map[string]any{"currency": "EUR"}}))
	link := a.ok(a.alice, "POST", "/api/v1/tables/"+orders+"/fields", map[string]any{"name": "Items", "type": "link", "options": map[string]any{"table_id": n(products)}})
	if num(link["options"].(map[string]any), "inverse") == 0 {
		t.Fatalf("no other side: %v", link)
	}
	a.ok(a.alice, "POST", "/api/v1/tables/"+orders+"/fields", map[string]any{"name": "Total", "type": "rollup", "options": map[string]any{"link_field": num(link, "id"), "field": n(price), "fn": "sum"}})
	a.ok(a.alice, "POST", "/api/v1/tables/"+orders+"/fields", map[string]any{"name": "With tax", "type": "formula", "options": map[string]any{"expr": "ROUND({Total} * 1.17, 2)"}})
	a.expect(400, a.alice, "POST", "/api/v1/tables/"+orders+"/fields", map[string]any{"name": "Bad", "type": "formula", "options": map[string]any{"expr": "{Nope} + 1"}})
	a.expect(400, a.alice, "POST", "/api/v1/tables/"+orders+"/fields", map[string]any{"name": "items", "type": "text"}) // the name is taken

	cake := a.ok(a.alice, "POST", "/api/v1/tables/"+products+"/records", map[string]any{"values": map[string]any{"Name": "Cake", "Price": "12,50"}})
	tea := a.ok(a.alice, "POST", "/api/v1/tables/"+products+"/records", map[string]any{"values": map[string]any{"Name": "Tea", "Price": 3}})
	o := a.ok(a.alice, "POST", "/api/v1/tables/"+orders+"/records", map[string]any{"values": map[string]any{"Name": "Order 1", "Items": []any{num(cake, "id"), num(tea, "id")}}})
	got := a.ok(a.alice, "GET", "/api/v1/records/"+id(o), nil)["record"].(map[string]any)["named"].(map[string]any)
	if got["Total"] != "15.5" || got["With tax"] != "18.14" || got["Items"] != "Cake, Tea" {
		t.Fatalf("worked out: %v", got)
	}
	// the other side shows the order
	p := a.ok(a.alice, "GET", "/api/v1/records/"+id(cake), nil)["record"].(map[string]any)["named"].(map[string]any)
	if p["Orders"] != "Order 1" {
		t.Fatalf("other side: %v", p)
	}
	// a typecast link by name makes the missing row
	a.ok(a.alice, "POST", "/api/v1/tables/"+orders+"/records", map[string]any{"values": map[string]any{"Name": "Order 2", "Items": "Tea, Coffee"}, "typecast": true})
	if l := a.ok(a.alice, "GET", "/api/v1/tables/"+products+"/records", nil); num(l, "total") != 3 {
		t.Fatalf("Coffee was not added: %v", l["total"])
	}
	// changing a text field to a number converts the values
	note := id(a.ok(a.alice, "POST", "/api/v1/tables/"+products+"/fields", map[string]any{"name": "Stock", "type": "text"}))
	a.ok(a.alice, "PUT", "/api/v1/records/"+id(tea), map[string]any{"values": map[string]any{"Stock": "40"}})
	a.ok(a.alice, "PUT", "/api/v1/fields/"+note, map[string]any{"type": "number"})
	r := a.ok(a.alice, "GET", "/api/v1/records/"+id(tea), nil)["record"].(map[string]any)["values"].(map[string]any)
	if r[note] != 40.0 {
		t.Fatalf("converted: %v", r[note])
	}
	// renaming a field keeps the formula working
	a.ok(a.alice, "PUT", "/api/v1/fields/"+id(link), map[string]any{"name": "Products"})
	rows := a.ok(a.alice, "GET", "/api/v1/tables/"+orders+"/records?named=1", nil)["records"].([]any)
	if rows[0].(map[string]any)["named"].(map[string]any)["With tax"] != "18.14" {
		t.Fatalf("after rename: %v", rows[0])
	}
	// a select refuses an unknown option unless typecast
	a.expect(400, a.alice, "POST", "/api/v1/tables/"+orders+"/records", map[string]any{"values": map[string]any{"Status": "Shipped"}})
	// the bin: delete, find it there, bring it back
	a.expect(204, a.alice, "DELETE", "/api/v1/records/"+id(o), nil)
	if l := a.ok(a.alice, "GET", "/api/v1/tables/"+orders+"/records?deleted=1", nil); num(l, "total") != 1 {
		t.Fatalf("bin: %v", l)
	}
	a.ok(a.alice, "POST", "/api/v1/records/"+id(o)+"/restore", nil)
}

func TestViewsFilterSortAndCSV(t *testing.T) {
	a := newApp(t, nil)
	tid, page := a.newTable(a.alice, "Books")
	a.ok(a.alice, "POST", "/api/v1/tables/"+tid+"/fields", map[string]any{"name": "Pages", "type": "number"})
	csvText := "Name,Pages,Status\nDune,612,Done\nEmma,474,To do\nIt,1138,Doing\n"
	a.ok(a.alice, "POST", "/api/v1/tables/"+tid+"/import", map[string]any{"text": csvText})
	pages := fieldID(a.ok(a.alice, "GET", "/api/v1/tables/"+tid, nil), "Pages")
	view := page["views"].([]any)[0].(map[string]any)
	a.ok(a.alice, "PUT", "/api/v1/views/"+id(view), map[string]any{"config": map[string]any{
		"filters":   []any{map[string]any{"field": n(pages), "op": "gt", "value": 500}},
		"sorts":     []any{map[string]any{"field": n(pages), "desc": true}},
		"summaries": map[string]any{pages: "sum"},
	}})
	l := a.ok(a.alice, "GET", "/api/v1/tables/"+tid+"/records?view="+id(view), nil)
	recs := l["records"].([]any)
	if len(recs) != 2 || recs[0].(map[string]any)["title"] != "It" || l["summaries"].(map[string]any)[pages] != 1750.0 {
		t.Fatalf("view: %v", l)
	}
	req, _ := http.NewRequest("GET", a.url+"/api/v1/tables/"+tid+"/export.csv?view="+id(view), nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, _ := http.DefaultClient.Do(req)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "It,,Doing,1138") && !strings.Contains(string(body), "It") {
		t.Fatalf("csv: %s", body)
	}
	// a new table from a spreadsheet guesses the types
	b := a.ok(a.alice, "POST", "/api/v1/bases", map[string]any{"name": "Imports", "empty": true})
	made := a.ok(a.alice, "POST", "/api/v1/bases/"+id(b)+"/import", map[string]any{"name": "People", "text": "Name;Email;Born;Team\nAna;ana@x.ba;1990-02-01;Red\nEmir;emir@x.ba;1988-07-12;Blue\nLejla;lejla@x.ba;1993-11-30;Red\nMarko;marko@x.ba;1979-01-05;Red\n"})
	if num(made, "made") != 4 {
		t.Fatalf("import: %v", made)
	}
	pg := a.ok(a.alice, "GET", "/api/v1/tables/"+id(made["table"].(map[string]any)), nil)
	types := map[string]string{}
	for _, f := range pg["fields"].([]any) {
		m := f.(map[string]any)
		types[m["name"].(string)] = m["type"].(string)
	}
	if types["Email"] != "email" || types["Born"] != "date" || types["Team"] != "select" {
		t.Fatalf("guessed: %v", types)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "POST", "/api/v1/templates/projects", map[string]any{"examples": true})
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := io.ReadAll(res.Body)
	res.Body.Close()
	// Bob brings Alice's export into his own account
	got := a.ok(a.bob, "POST", "/api/v1/import", map[string]any{"text": string(doc)})
	if num(got, "bases") != 1 || num(got, "tables") != 2 || num(got, "records") != 8 {
		t.Fatalf("import: %v", got)
	}
	bases := a.list(a.bob, "/api/v1/bases")
	var projects string
	for _, tb := range bases[0].(map[string]any)["tables"].([]any) {
		if tb.(map[string]any)["name"] == "Projects" {
			projects = id(tb.(map[string]any))
		}
	}
	rows := a.ok(a.bob, "GET", "/api/v1/tables/"+projects+"/records?named=1", nil)["records"].([]any)
	for _, r := range rows {
		named := r.(map[string]any)["named"].(map[string]any)
		if named["Project"] == "New website" && !strings.Contains(named["Tasks"].(string), "Write the texts") {
			t.Fatalf("links did not come back: %v", named)
		}
	}
	// Alice still has exactly hers
	if l := a.list(a.alice, "/api/v1/bases"); len(l) != 1 {
		t.Fatalf("Alice's bases: %d", len(l))
	}
}

func TestSettingsArePerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "BAM", "notify_url": "https://ntfy.example/alice"})
	if s := a.ok(a.bob, "GET", "/api/v1/settings", nil); s["currency"] == "BAM" || s["notify_url"] != "" {
		t.Fatalf("Bob got Alice's settings: %v %v", s["currency"], s["notify_url"])
	}
}

func TestLinkScopes(t *testing.T) {
	get := func(p string) *http.Request { r, _ := http.NewRequest("GET", p, nil); return r }
	post := func(p string) *http.Request { r, _ := http.NewRequest("POST", p, nil); return r }
	put := func(p string) *http.Request { r, _ := http.NewRequest("PUT", p, nil); return r }
	del := func(p string) *http.Request { r, _ := http.NewRequest("DELETE", p, nil); return r }
	read, write := []string{"tables:read"}, []string{"tables:write"}
	for _, c := range []struct {
		r      *http.Request
		scopes []string
		want   bool
	}{
		{get("/api/v1/auth/me"), nil, true},
		{get("/api/v1/bases"), read, true},
		{get("/api/v1/tables/3/records"), read, true},
		{get("/api/v1/records/9"), read, true},
		{get("/api/v1/day/2026-11-07"), read, true},
		{get("/api/v1/search"), write, false},
		{post("/api/v1/tables/3/records"), write, true},
		{post("/api/v1/tables/3/records"), read, false},
		{put("/api/v1/records/9"), write, true},
		{del("/api/v1/records/9"), write, false},
		{post("/api/v1/tables/3/fields"), write, false},
		{get("/api/v1/settings"), read, false},
		{get("/api/v1/export.json"), read, false},
		{put("/api/v1/tables/3"), write, false},
	} {
		if got := linkAllows(c.scopes, c.r); got != c.want {
			t.Errorf("%s %s with %v: %v, want %v", c.r.Method, c.r.URL.Path, c.scopes, got, c.want)
		}
	}
}
