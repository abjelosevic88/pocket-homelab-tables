package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- a table with every value worked out ----

// loaded is one table as a request needs it.
type loaded struct {
	sheet  *store.Sheet
	fields []*tables.Field
	recs   []*store.Record
	byID   map[int64]*store.Record
	eng    *tables.Sheet
}

// loader reads tables once per request: a table and the tables its links,
// lookups and rollups read from (two steps deep; the farthest ones show
// their own values but no links of their own).
type loader struct {
	s    *Server
	uid  int64
	memo map[string]*loaded
	now  time.Time
}

func (s *Server) loader(uid int64) *loader {
	return &loader{s: s, uid: uid, memo: map[string]*loaded{}, now: time.Now()}
}

func (l *loader) load(ctx context.Context, sheetID int64, deleted bool) (*loaded, error) {
	return l.loadDepth(ctx, sheetID, deleted, 0)
}

func (l *loader) loadDepth(ctx context.Context, sheetID int64, deleted bool, depth int) (*loaded, error) {
	key := fmt.Sprintf("%d %v %d", sheetID, deleted, depth)
	if ld, ok := l.memo[key]; ok {
		return ld, nil
	}
	sh, err := l.s.store.GetSheet(ctx, l.uid, sheetID)
	if err != nil {
		return nil, err
	}
	fields, err := l.s.store.Fields(ctx, l.uid, sheetID)
	if err != nil {
		return nil, err
	}
	recs, err := l.s.store.Records(ctx, l.uid, sheetID, deleted)
	if err != nil {
		return nil, err
	}
	ld := &loaded{sheet: sh, fields: fields, recs: recs, byID: map[int64]*store.Record{}}
	eng := &tables.Sheet{Fields: fields, Linked: map[int64]*tables.Sheet{}, Now: l.now}
	er := map[int64]*tables.Record{}
	for _, r := range recs {
		ld.byID[r.ID] = r
		x := &tables.Record{ID: r.ID, Number: r.Number, Data: r.Data, Links: map[int64][]tables.LinkRef{}, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
		eng.Records = append(eng.Records, x)
		er[r.ID] = x
	}
	if depth < 2 {
		// the stored links: under the field itself, or under the other side's field
		var ids []int64
		for _, f := range fields {
			if f.Type != tables.Link {
				continue
			}
			if f.Options.Reverse {
				ids = append(ids, f.Options.Inverse)
			} else {
				ids = append(ids, f.ID)
			}
		}
		rowsOf, err := l.s.store.Links(ctx, l.uid, ids)
		if err != nil {
			return nil, err
		}
		titles := map[int64]*loaded{}
		for _, f := range fields {
			if f.Type != tables.Link || f.Options.TableID == 0 {
				continue
			}
			other := titles[f.Options.TableID]
			if other == nil {
				if f.Options.TableID == sheetID && depth > 0 {
					continue
				}
				other, err = l.loadDepth(ctx, f.Options.TableID, false, depth+1)
				if err != nil {
					continue // the other table is gone
				}
				titles[f.Options.TableID] = other
				eng.Linked[f.Options.TableID] = other.eng
			}
			for _, lr := range rowsOf {
				var mine, theirs int64
				switch {
				case !f.Options.Reverse && lr.FieldID == f.ID:
					mine, theirs = lr.From, lr.To
				case f.Options.Reverse && lr.FieldID == f.Options.Inverse:
					mine, theirs = lr.To, lr.From
				default:
					continue
				}
				x := er[mine]
				o := other.eng.Record(theirs)
				if x == nil || o == nil {
					continue
				}
				x.Links[f.ID] = append(x.Links[f.ID], tables.LinkRef{ID: theirs, Title: titleOr(other.eng.Title(o), o.Number)})
			}
		}
	}
	eng.Compute()
	ld.eng = eng
	l.memo[key] = ld
	return ld, nil
}

func titleOr(t string, n int64) string {
	if strings.TrimSpace(t) != "" {
		return t
	}
	return "Row " + strconv.FormatInt(n, 10)
}

// field finds a field of the table by id, or by name (not case-sensitive).
func (ld *loaded) field(key string) *tables.Field {
	if id, err := strconv.ParseInt(key, 10, 64); err == nil {
		for _, f := range ld.fields {
			if f.ID == id {
				return f
			}
		}
	}
	for _, f := range ld.fields {
		if strings.EqualFold(strings.TrimSpace(f.Name), strings.TrimSpace(key)) {
			return f
		}
	}
	return nil
}

func (ld *loaded) fieldByID(id int64) *tables.Field {
	for _, f := range ld.fields {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// ---- rows as the page and the other apps see them ----

type recordOut struct {
	ID        int64          `json:"id"`
	TableID   int64          `json:"table_id"`
	Number    int64          `json:"number"`
	Title     string         `json:"title"`
	Values    map[string]any `json:"values"`
	Source    string         `json:"source"`
	Comments  int            `json:"comments"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	DeletedAt string         `json:"deleted_at,omitempty"`
	// Named are the values as text by field name, for the other apps.
	Named map[string]string `json:"named,omitempty"`
}

func (ld *loaded) out(x *tables.Record, named bool) recordOut {
	r := ld.byID[x.ID]
	o := recordOut{ID: x.ID, TableID: ld.sheet.ID, Number: x.Number, Title: titleOr(ld.eng.Title(x), x.Number), Values: x.Values, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt}
	if o.Values == nil {
		o.Values = map[string]any{}
	}
	if r != nil {
		o.Source, o.DeletedAt = r.Source, r.DeletedAt
	}
	if named {
		o.Named = ld.named(x)
	}
	return o
}

// named is a row's values as text by field name.
func (ld *loaded) named(x *tables.Record) map[string]string {
	m := map[string]string{}
	for _, f := range ld.fields {
		if t := ld.eng.Text(x, f); t != "" {
			m[f.Name] = t
		}
	}
	return m
}

// view keeps the rows a view shows, in its order; q searches every field.
func (ld *loaded) view(v *store.View, q string, now time.Time) []*tables.Record {
	recs := ld.eng.Search(q)
	if v == nil {
		return recs
	}
	out := make([]*tables.Record, 0, len(recs))
	for _, r := range recs {
		if ld.eng.Match(r, v.Config.Filters, v.Config.Match == "any", now) {
			out = append(out, r)
		}
	}
	sorts := v.Config.Sorts
	if v.Type == "grid" && v.Config.GroupBy > 0 {
		has := false
		for _, so := range sorts {
			has = has || so.Field == v.Config.GroupBy
		}
		if !has {
			sorts = append([]tables.Sort{{Field: v.Config.GroupBy}}, sorts...)
		}
	}
	ld.eng.SortRecords(out, sorts)
	return out
}

// ---- writing values ----

// linkSet is the rows one link field of a row points at.
type linkSet struct {
	f   *tables.Field
	ids []int64
}

// change is one field changed on a row, for its story.
type change struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// setValues applies values (by field id or name) to a row in memory and
// stores links. lenient takes what fits and adds unknown options (imports,
// other apps, AI); otherwise a value that does not fit is refused.
// It returns what changed; the caller saves the row.
func (s *Server) setValues(ctx context.Context, uid int64, ld *loaded, r *store.Record, in map[string]any, lenient bool) ([]change, []linkSet, error) {
	var changes []change
	var links, pending []linkSet
	var fieldsChanged []*tables.Field
	for key, raw := range in {
		f := ld.field(key)
		if f == nil {
			if lenient {
				continue
			}
			return nil, nil, &userErr{"there is no field " + strconv.Quote(key)}
		}
		if tables.Computed(f.Type) {
			if lenient {
				continue
			}
			return nil, nil, &userErr{f.Name + " is worked out by itself and cannot be typed in"}
		}
		if f.Type == tables.Link {
			ids, err := s.linkIDs(ctx, uid, ld, f, raw, lenient)
			if err != nil {
				return nil, nil, err
			}
			links = append(links, linkSet{f, ids})
			continue
		}
		before := len(f.Options.Choices)
		v, err := tables.Normalize(f, raw, lenient)
		if err != nil {
			return nil, nil, &userErr{err.Error()}
		}
		if len(f.Options.Choices) != before {
			fieldsChanged = append(fieldsChanged, f)
		}
		k := tables.Key(f.ID)
		old := r.Data[k]
		bt, at := tables.Display(f, old), tables.Display(f, v)
		if v == nil {
			delete(r.Data, k)
		} else {
			r.Data[k] = v
		}
		if bt != at {
			changes = append(changes, change{f.Name, clip(bt, 200), clip(at, 200)})
		}
	}
	for _, f := range fieldsChanged {
		if err := s.store.UpdateField(ctx, uid, f); err != nil {
			return nil, nil, err
		}
	}
	for _, ls := range links {
		stored, reverse := ls.f.ID, false
		if ls.f.Options.Reverse {
			stored, reverse = ls.f.Options.Inverse, true
		}
		if ls.f.Options.Single && len(ls.ids) > 1 {
			ls.ids = ls.ids[:1]
		}
		var before []string
		if x := ld.eng.Record(r.ID); x != nil {
			for _, l := range x.Links[ls.f.ID] {
				before = append(before, l.Title)
			}
		}
		if r.ID > 0 {
			if err := s.store.SetLinks(ctx, uid, stored, r.ID, ls.ids, reverse); err != nil {
				return nil, nil, err
			}
		} else {
			pending = append(pending, ls) // stored once the row has an id
		}
		changes = append(changes, change{ls.f.Name, clip(strings.Join(before, ", "), 200), fmt.Sprintf("%d linked", len(ls.ids))})
	}
	return changes, pending, nil
}

// flushLinks stores the links set on a row before it had an id.
func (s *Server) flushLinks(ctx context.Context, uid int64, r *store.Record, pending []linkSet) error {
	for _, ls := range pending {
		stored, reverse := ls.f.ID, false
		if ls.f.Options.Reverse {
			stored, reverse = ls.f.Options.Inverse, true
		}
		if err := s.store.SetLinks(ctx, uid, stored, r.ID, ls.ids, reverse); err != nil {
			return err
		}
	}
	return nil
}

// linkIDs reads the rows a link should point at: ids, {id} objects, or
// (lenient) the rows' names in the other table — unknown names become new rows there.
func (s *Server) linkIDs(ctx context.Context, uid int64, ld *loaded, f *tables.Field, raw any, lenient bool) ([]int64, error) {
	var items []any
	switch t := raw.(type) {
	case nil:
		return []int64{}, nil
	case []any:
		items = t
	case string:
		if strings.TrimSpace(t) == "" {
			return []int64{}, nil
		}
		for _, p := range strings.Split(t, ",") {
			items = append(items, strings.TrimSpace(p))
		}
	default:
		items = []any{t}
	}
	var ids []int64
	var names []string
	for _, it := range items {
		switch t := it.(type) {
		case float64:
			ids = append(ids, int64(t))
		case map[string]any:
			if n, ok := tables.ToNumber(t["id"]); ok {
				ids = append(ids, int64(n))
			} else if s := tables.ToText(t["title"]); s != "" {
				names = append(names, s)
			}
		case string:
			if n, err := strconv.ParseInt(t, 10, 64); err == nil && !lenient {
				ids = append(ids, n)
			} else if t != "" {
				names = append(names, t)
			}
		}
	}
	ids = s.store.OwnRecords(ctx, uid, f.Options.TableID, ids)
	if len(names) > 0 {
		if !lenient {
			return nil, &userErr{f.Name + ": pick rows from the list"}
		}
		other, err := s.loader(uid).load(ctx, f.Options.TableID, false)
		if err != nil {
			return ids, nil
		}
		for _, n := range names {
			found := int64(0)
			for _, x := range other.eng.Records {
				if strings.EqualFold(strings.TrimSpace(other.eng.Title(x)), strings.TrimSpace(n)) {
					found = x.ID
					break
				}
			}
			if found == 0 {
				p := other.eng.Primary()
				if p == nil || tables.Computed(p.Type) {
					continue
				}
				v, _ := tables.Normalize(p, n, true)
				nr := &store.Record{UserID: uid, SheetID: f.Options.TableID, Data: map[string]any{tables.Key(p.ID): v}, Source: "tables"}
				if err := s.store.CreateRecord(ctx, nr); err != nil {
					continue
				}
				other.eng.Records = append(other.eng.Records, &tables.Record{ID: nr.ID, Data: nr.Data, Values: map[string]any{tables.Key(p.ID): v}})
				found = nr.ID
			}
			ids = append(ids, found)
		}
	}
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// ---- telling the box ----

func (s *Server) recordLink(id int64) string {
	if s.cfg.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/r/" + strconv.FormatInt(id, 10)
}

func (s *Server) tableLink(id int64) string {
	if s.cfg.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/t/" + strconv.FormatInt(id, 10)
}

// recordEvent publishes a row's event with its values by field name.
func (s *Server) recordEvent(ctx context.Context, uid int64, topic string, ld *loaded, id int64, changes []change) {
	if !s.core.Configured() {
		return
	}
	payload := map[string]any{"table_id": ld.sheet.ID, "table": ld.sheet.Name, "record_id": id, "url": s.recordLink(id)}
	if b, err := s.store.GetBase(ctx, uid, ld.sheet.BaseID); err == nil {
		payload["base"] = b.Name
	}
	subject := ld.sheet.Name
	if x := ld.eng.Record(id); x != nil {
		payload["title"] = titleOr(ld.eng.Title(x), x.Number)
		payload["number"] = x.Number
		payload["values"] = ld.named(x)
		subject = ld.sheet.Name + ": " + titleOr(ld.eng.Title(x), x.Number)
	}
	if len(changes) > 0 {
		names := make([]string, 0, len(changes))
		for _, c := range changes {
			names = append(names, c.Field)
		}
		sort.Strings(names)
		payload["changed"] = names
		payload["changes"] = changes
	}
	s.publish(ctx, uid, topic, subject, payload)
}

// ---- small helpers ----

type userErr struct{ msg string }

func (e *userErr) Error() string { return e.msg }

// userFail answers a mistake in what was sent with 400, else the usual.
func (s *Server) userFail(w http.ResponseWriter, err error, what string) {
	var ue *userErr
	var ie *tables.InputError
	switch {
	case asErr(err, &ue):
		writeErr(w, http.StatusBadRequest, ue.msg)
	case asErr(err, &ie):
		writeErr(w, http.StatusBadRequest, ie.Msg)
	default:
		s.fail(w, err, what)
	}
}

func today() string { return time.Now().Format("2006-01-02") }

func (s *Server) settingsOf(ctx context.Context, uid int64) store.Settings {
	st, _ := s.store.GetSettings(ctx, uid)
	return st
}
