package server

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- across all tables: home, search, a day, a person ----
//
// For the home page and for the rest of the box: Pocket Day (rows on a
// day), Search, Contacts (rows that mention a person) and Pocket AI.

type hit struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	TableID   int64  `json:"table_id"`
	Table     string `json:"table"`
	TableIcon string `json:"table_icon,omitempty"`
	Base      string `json:"base"`
	Field     string `json:"field,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Date      string `json:"date,omitempty"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// everything loads every table of a person, worked out.
func (s *Server) everything(ctx context.Context, uid int64) ([]*loaded, map[int64]*store.Base, time.Time, error) {
	bases, err := s.store.ListBases(ctx, uid)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	bm := map[int64]*store.Base{}
	for _, b := range bases {
		bm[b.ID] = b
	}
	sheets, err := s.store.ListSheets(ctx, uid, 0)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	l := s.loader(uid)
	var out []*loaded
	for _, t := range sheets {
		ld, err := l.load(ctx, t.ID, false)
		if err != nil {
			continue
		}
		out = append(out, ld)
	}
	return out, bm, l.now, nil
}

func (s *Server) hitOf(ld *loaded, bm map[int64]*store.Base, x *tables.Record) hit {
	h := hit{ID: x.ID, Title: titleOr(ld.eng.Title(x), x.Number), TableID: ld.sheet.ID, Table: ld.sheet.Name, TableIcon: ld.sheet.Icon, URL: s.recordLink(x.ID), UpdatedAt: x.UpdatedAt}
	if b := bm[ld.sheet.BaseID]; b != nil {
		h.Base = b.Name
	}
	return h
}

// due lists rows whose reminder dates fall between from and to (YYYY-MM-DD).
func (s *Server) due(all []*loaded, bm map[int64]*store.Base, from, to string, onlyRemind bool) []hit {
	out := []hit{}
	for _, ld := range all {
		for _, f := range ld.fields {
			if f.Type != tables.Date || (onlyRemind && !f.Options.Remind) {
				continue
			}
			for _, x := range ld.eng.Records {
				d := tables.ToText(x.Values[tables.Key(f.ID)])
				if len(d) < 10 || d[:10] < from || d[:10] > to {
					continue
				}
				h := s.hitOf(ld, bm, x)
				h.Field, h.Date = f.Name, d
				out = append(out, h)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	all, bm, now, err := s.everything(ctx, uid)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	rows := 0
	var recent []hit
	for _, ld := range all {
		rows += len(ld.eng.Records)
		for _, x := range ld.eng.Records {
			recent = append(recent, s.hitOf(ld, bm, x))
		}
	}
	sort.Slice(recent, func(i, j int) bool { return recent[i].UpdatedAt > recent[j].UpdatedAt })
	if len(recent) > 10 {
		recent = recent[:10]
	}
	t := now.Format("2006-01-02")
	soon := s.due(all, bm, t, now.AddDate(0, 0, 7).Format("2006-01-02"), true)
	late := s.due(all, bm, now.AddDate(0, 0, -30).Format("2006-01-02"), now.AddDate(0, 0, -1).Format("2006-01-02"), true)
	today := 0
	for _, h := range soon {
		if h.Date[:10] == t {
			today++
		}
	}
	if recent == nil {
		recent = []hit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"bases": len(bm), "tables": len(all), "rows": rows, "due_today": today, "soon": soon, "late": late, "recent": recent})
}

// handleSearch finds rows with the words in any field, and tables by name.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	out := map[string]any{"query": q, "items": []hit{}, "tables": []map[string]any{}}
	if q == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	all, bm, _, err := s.everything(ctx, uid)
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	lq := strings.ToLower(q)
	items := []hit{}
	tbls := []map[string]any{}
	for _, ld := range all {
		if strings.Contains(strings.ToLower(ld.sheet.Name), lq) {
			b := bm[ld.sheet.BaseID]
			tbls = append(tbls, map[string]any{"id": ld.sheet.ID, "name": ld.sheet.Name, "icon": ld.sheet.Icon, "base": b.Name, "rows": len(ld.eng.Records), "url": s.tableLink(ld.sheet.ID)})
		}
		for _, x := range ld.eng.Records {
			if len(items) >= limit {
				break
			}
			for _, f := range ld.fields {
				t := ld.eng.Text(x, f)
				i := strings.Index(strings.ToLower(t), lq)
				if i < 0 {
					continue
				}
				h := s.hitOf(ld, bm, x)
				if !f.Primary {
					h.Field, h.Snippet = f.Name, snippet(t, i, len(q))
				}
				items = append(items, h)
				break
			}
		}
	}
	out["items"], out["tables"] = items, tbls
	writeJSON(w, http.StatusOK, out)
}

func snippet(t string, i, n int) string {
	start := max(0, i-40)
	end := min(len(t), i+n+60)
	for start > 0 && !utf8Start(t[start]) {
		start--
	}
	for end < len(t) && !utf8Start(t[end]) {
		end++
	}
	s := strings.ReplaceAll(t[start:end], "\n", " ")
	if start > 0 {
		s = "…" + s
	}
	if end < len(t) {
		s += "…"
	}
	return s
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// handleLookup finds the rows about a person: an email in an email field,
// a contact picked in an "In Pocket" field, or a name.
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	contact := strings.TrimSpace(r.URL.Query().Get("contact"))
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	items := []hit{}
	if email == "" && contact == "" && len(name) < 3 {
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	all, bm, _, err := s.everything(ctx, uid)
	if err != nil {
		s.fail(w, err, "lookup")
		return
	}
	for _, ld := range all {
		for _, x := range ld.eng.Records {
			for _, f := range ld.fields {
				v := x.Values[tables.Key(f.ID)]
				ok := false
				switch f.Type {
				case tables.Email:
					ok = email != "" && strings.EqualFold(tables.ToText(v), email)
				case tables.Pocket:
					if refs, _ := tables.Normalize(f, v, true); refs != nil {
						for _, ref := range refs.([]tables.PocketRef) {
							ok = ok || (contact != "" && ref.App == "contact" && ref.Ref == contact) || (name != "" && ref.App == "contact" && strings.EqualFold(ref.Title, name))
						}
					}
				case tables.Text:
					ok = name != "" && strings.EqualFold(strings.TrimSpace(tables.ToText(v)), name)
				}
				if ok {
					h := s.hitOf(ld, bm, x)
					h.Field = f.Name
					items = append(items, h)
					break
				}
			}
			if len(items) >= 50 {
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleDay lists the rows with a date on a day (for Pocket Day and the calendar).
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d := chiParam(r, "date")
	if !validDate(d) {
		writeErr(w, http.StatusBadRequest, "the day is YYYY-MM-DD")
		return
	}
	all, bm, _, err := s.everything(ctx, uid)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": d, "items": s.due(all, bm, d, d, false)})
}

// handleToday is the home page's list: reminders today, overdue and the next week.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	all, bm, now, err := s.everything(ctx, uid)
	if err != nil {
		s.fail(w, err, "today")
		return
	}
	t := now.Format("2006-01-02")
	writeJSON(w, http.StatusOK, map[string]any{"date": t,
		"today": s.due(all, bm, t, t, true),
		"late":  s.due(all, bm, now.AddDate(0, 0, -30).Format("2006-01-02"), now.AddDate(0, 0, -1).Format("2006-01-02"), true),
		"soon":  s.due(all, bm, now.AddDate(0, 0, 1).Format("2006-01-02"), now.AddDate(0, 0, 7).Format("2006-01-02"), true)})
}
