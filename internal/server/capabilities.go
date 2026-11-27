package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Tables (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They read what /search, /day and the first page read, as the request's
// person, and change nothing. There is no quick line: a row belongs to a
// table and its fields have types, and nobody has a table new rows go to.

const (
	searchPool   = 200 // rows looked at before the closest are picked: the most /search gives
	lineValues   = 3   // values of a row in its line
	lineValueLen = 60  // a long value is cut
	widgetTables = 5
	widgetDays   = 7
)

// rowsWord: "1 row", "5 rows".
func rowsWord(n int) string {
	return fmt.Sprintf("%d %s", n, plural(n, "row", "rows"))
}

// dots joins the parts of a line that are there: "Clients in CRM · City: Sarajevo".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// hasWord reports whether there is a letter or a digit to look for.
func hasWord(q string) bool {
	return strings.IndexFunc(q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// tableNames: "Books", "Books and Clients", "Books, Clients and Orders",
// "Books, Clients and 3 other tables" — each name once, in the order given.
func tableNames(names []string) string {
	uniq := []string{}
	for _, n := range names {
		if n != "" && !oneOf(n, uniq) {
			uniq = append(uniq, n)
		}
	}
	switch n := len(uniq); {
	case n == 0:
		return ""
	case n == 1:
		return uniq[0]
	case n > 3:
		return fmt.Sprintf("%s, %s and %d other tables", uniq[0], uniq[1], n-2)
	}
	return strings.Join(uniq[:len(uniq)-1], ", ") + " and " + uniq[len(uniq)-1]
}

// ---------- search ----------

// inLine reports whether a field's value belongs in a row's line: files and
// the times and numbers a row gets on its own say nothing there.
func inLine(f *tables.Field) bool {
	switch f.Type {
	case tables.Attachment, tables.Created, tables.Updated, tables.AutoNumber:
		return false
	}
	return !f.Primary
}

// rowLine is a found row in one plain line: its table (and base), the field
// the words were found in, then its first few values. "Clients in CRM ·
// Email: ana@pekara.ba · City: Sarajevo". Empty values are left out.
func rowLine(m rowMatch, base, q string) string {
	where := m.ld.sheet.Name
	if base != "" && base != where {
		where += " in " + base
	}
	parts := []string{where}
	if !m.field.Primary {
		parts = append(parts, m.field.Name+": "+snippet(m.text, m.at, len(q)))
	}
	for _, f := range m.ld.fields {
		if len(parts) > lineValues {
			break
		}
		if f == m.field || !inLine(f) {
			continue
		}
		if v := strings.Join(strings.Fields(m.ld.eng.Text(m.rec, f)), " "); v != "" {
			parts = append(parts, f.Name+": "+clip(v, lineValueLen))
		}
	}
	return dots(parts...)
}

// pocketSearch answers Pocket Search: the tables and rows /search finds, as
// one list — tables with the words in their name first, then the rows named
// by them, then the rows that have them in another field.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if !hasWord(q) {
		return nil, nil
	}
	ctx := r.Context()
	rows, named, bm, err := s.find(ctx, actorID(ctx), q, searchPool)
	if err != nil {
		return nil, err
	}
	baseOf := func(ld *loaded) string {
		if b := bm[ld.sheet.BaseID]; b != nil {
			return b.Name
		}
		return ""
	}
	lq := strings.ToLower(q)
	out := []core.SearchItem{}
	for _, ld := range named {
		in := ""
		if b := baseOf(ld); b != "" {
			in = "In " + b
		}
		out = append(out, core.SearchItem{ID: "table:" + itoa(ld.sheet.ID), Title: ld.sheet.Name, Kind: "table", Date: ld.sheet.UpdatedAt,
			URL: "/t/" + itoa(ld.sheet.ID), Snippet: dots(in, rowsWord(len(ld.eng.Records))), Score: 3})
	}
	for _, m := range rows {
		it := core.SearchItem{ID: "row:" + itoa(m.rec.ID), Title: titleOr(m.ld.eng.Title(m.rec), m.rec.Number), Kind: "row", Date: m.rec.UpdatedAt,
			URL: "/r/" + itoa(m.rec.ID), Snippet: rowLine(m, baseOf(m.ld), q), Score: 1}
		if strings.Contains(strings.ToLower(it.Title), lq) {
			it.Score = 2
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- day ----------

// pocketDay answers Pocket Day: the rows added that day, the rows changed
// that day, and the rows /day reports — those with a date on it (a visit, a
// deadline). A row comes once: added before changed, changed before dated.
// "Added" and "changed" are the row's own times, as the "Added" and
// "Changed" fields show them, so a row changed again later counts for the
// later day only.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx := r.Context()
	all, bm, _, err := s.everything(ctx, actorID(ctx))
	if err != nil {
		return nil, err
	}
	if tz == nil {
		tz = time.Local
	}
	start, err := time.ParseInLocation("2006-01-02", date, tz)
	if err != nil {
		return nil, err
	}
	end := start.AddDate(0, 0, 1)
	// at is the time of day of a stored moment that falls on the day
	at := func(moment string) (string, bool) {
		t, err := time.Parse(time.RFC3339Nano, moment)
		if err != nil || t.Before(start) || !t.Before(end) {
			return "", false
		}
		return t.In(tz).Format("15:04"), true
	}
	var added, changed, dated []core.DayItem
	var addedIn, changedIn, datedIn []string
	seen := map[int64]bool{}
	for _, ld := range all {
		for _, x := range ld.eng.Records {
			it := core.DayItem{ID: "row:" + itoa(x.ID), Title: titleOr(ld.eng.Title(x), x.Number), Sub: ld.sheet.Name, URL: "/r/" + itoa(x.ID)}
			if t, ok := at(x.CreatedAt); ok {
				it.Time, it.Kind = t, "added"
				added, addedIn = append(added, it), append(addedIn, ld.sheet.Name)
			} else if t, ok := at(x.UpdatedAt); ok {
				it.Time, it.Kind = t, "changed"
				changed, changedIn = append(changed, it), append(changedIn, ld.sheet.Name)
			} else {
				continue
			}
			seen[x.ID] = true
		}
	}
	for _, h := range s.due(all, bm, date, date, false) {
		if seen[h.ID] { // also a row with two dates on the day
			continue
		}
		seen[h.ID] = true
		it := core.DayItem{ID: "row:" + itoa(h.ID), Title: h.Title, Sub: dots(h.Table, h.Field), Kind: "dated", URL: "/r/" + itoa(h.ID)}
		if len(h.Date) >= 16 { // the date has a time: 2026-11-27T14:30
			it.Time = h.Date[11:16]
		}
		dated, datedIn = append(dated, it), append(datedIn, h.Table)
	}
	if len(seen) == 0 {
		return nil, nil
	}
	var parts []string
	for _, p := range []struct {
		n     int
		what  string
		names []string
	}{{len(added), "added in", addedIn}, {len(changed), "changed in", changedIn}, {len(dated), "with this date in", datedIn}} {
		if p.n > 0 {
			parts = append(parts, rowsWord(p.n)+" "+p.what+" "+tableNames(p.names))
		}
	}
	// the rows about the day first: a day of many new rows keeps them in sight
	return &core.Day{Date: date, Summary: strings.Join(parts, "; "), Items: append(append(dated, added...), changed...)}, nil
}

// ---------- widget ----------

// pocketWidget answers Pocket Hub: the tables touched last, each with how
// many of its rows were added or changed in the last week. It counts only
// the rows of that week; nothing is worked out. Someone without tables gets
// nothing to show.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	sheets, err := s.store.ListSheets(ctx, uid, 0)
	if err != nil {
		return nil, err
	}
	if len(sheets) == 0 {
		return nil, nil
	}
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	changes, err := s.store.ChangedSince(ctx, uid, now.AddDate(0, 0, -widgetDays), midnight)
	if err != nil {
		return nil, err
	}
	lately := map[int64]store.SheetChange{}
	today := 0
	for _, c := range changes {
		lately[c.SheetID] = c
		today += c.Today
	}
	// a table is touched when a row is added or changed, or the table itself is
	touched := map[int64]time.Time{}
	for _, t := range sheets {
		for _, moment := range []string{t.UpdatedAt, lately[t.ID].Last} {
			if at, err := time.Parse(time.RFC3339Nano, moment); err == nil && at.After(touched[t.ID]) {
				touched[t.ID] = at
			}
		}
	}
	sort.SliceStable(sheets, func(i, j int) bool { return touched[sheets[i].ID].After(touched[sheets[j].ID]) })
	if len(sheets) > widgetTables {
		sheets = sheets[:widgetTables]
	}
	w := &core.WidgetData{URL: "/"}
	if today > 0 {
		w.Line = rowsWord(today) + " added or changed today"
	}
	for _, t := range sheets {
		detail := "No rows yet"
		switch c := lately[t.ID]; {
		case c.Rows > 0:
			detail = rowsWord(c.Rows) + " added or changed in the last week"
		case t.Records > 0:
			detail = rowsWord(t.Records)
		}
		w.Items = append(w.Items, core.WidgetItem{ID: "table:" + itoa(t.ID), Title: t.Name, Detail: detail, URL: "/t/" + itoa(t.ID)})
	}
	return w, nil
}
