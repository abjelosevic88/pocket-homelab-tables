package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- things in the other apps a cell can point at ----
//
// A "In Pocket" field points at one kind of thing: a contact, a task, an
// event… The picker searches that app as the person (their Core link or
// direct token); the cell keeps the id, the title and a link, so it still
// reads well when the app is away. "Make a table from…" reads a whole list.

type importCol struct {
	key   string // the key in the app's JSON ("emails" takes the first)
	name  string
	typ   string
	cents bool // money in minor units
}

type pocketKind struct {
	slug   string // contact, task…
	label  string // Contact
	plural string // contacts
	icon   string
	app    pocketApp
	scope  string
	search string // path; q= and limit= are added
	keys   []string
	title  []string
	sub    func(m map[string]any) string
	open   func(id string, m map[string]any) string
	list   string // path for everything ("" = not offered)
	cols   []importCol
}

func dayOf(v any) string { return day10(v) }

func moneyText(m map[string]any, k string) string {
	n := num(m, k)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f %s", n/100, str(m, "currency"))
}

var pocketKinds = []pocketKind{
	{slug: "contact", label: "Contact", plural: "contacts", icon: "users", app: appContacts, scope: scopeContactsRead,
		search: "/api/v1/search", keys: []string{"contacts"}, title: []string{"fn", "name"},
		sub:  func(m map[string]any) string { return firstNonEmpty(firstValue(m["emails"]), str(m, "org")) },
		open: func(id string, _ map[string]any) string { return "/contacts/" + id },
		list: "/api/v1/contacts?limit=5000",
		cols: []importCol{{"fn", "Name", tables.Text, false}, {"emails", "Email", tables.Email, false}, {"phones", "Phone", tables.Phone, false},
			{"org", "Company", tables.Text, false}, {"title", "Job title", tables.Text, false}, {"birthday", "Birthday", tables.Date, false},
			{"categories", "Groups", tables.Multi, false}, {"starred", "Favourite", tables.Checkbox, false}}},
	{slug: "task", label: "Task", plural: "tasks", icon: "check-square", app: appTasks, scope: scopeTasksRead,
		search: "/api/v1/search", title: []string{"title"},
		sub:  func(m map[string]any) string { return joinNonEmpty(" · ", dayOf(m["due"]), str(m, "status")) },
		open: func(id string, _ map[string]any) string { return "/inbox?task=" + id },
		list: "/api/v1/tasks?limit=2000",
		cols: []importCol{{"title", "Task", tables.Text, false}, {"status", "Status", tables.Select, false}, {"due", "Due", tables.Date, false},
			{"start", "Start", tables.Date, false}, {"priority", "Priority", tables.Number, false}, {"tags", "Tags", tables.Multi, false}, {"notes", "Notes", tables.LongText, false}}},
	{slug: "event", label: "Event", plural: "events", icon: "calendar", app: appCalendar, scope: scopeCalendarRead,
		search: "/api/v1/search", title: []string{"title"},
		sub: func(m map[string]any) string {
			return joinNonEmpty(" · ", strings.Replace(clip(str(m, "start"), 16), "T", " ", 1), str(m, "location"))
		},
		open: func(id string, _ map[string]any) string { return "/event/" + id }},
	{slug: "note", label: "Note", plural: "notes", icon: "note", app: appNotes, scope: scopeNotesRead,
		search: "/api/v1/search", title: []string{"title"},
		sub:  func(m map[string]any) string { return firstNonEmpty(str(m, "category"), clip(str(m, "excerpt"), 80)) },
		open: func(id string, _ map[string]any) string { return "/note/" + id }},
	{slug: "document", label: "Document", plural: "documents", icon: "file", app: appDocuments, scope: scopeDocumentsRead,
		search: "/api/v1/documents", keys: []string{"results"}, title: []string{"title", "original_name"},
		sub:  func(m map[string]any) string { return dayOf(m["created"]) },
		open: func(id string, _ map[string]any) string { return "/documents/" + id }},
	{slug: "item", label: "Thing you own", plural: "inventory", icon: "package", app: appInventory, scope: scopeItemsRead,
		search: "/api/v1/items", title: []string{"name"},
		sub:  func(m map[string]any) string { return firstNonEmpty(str(m, "location_name"), str(m, "category_name")) },
		open: func(id string, _ map[string]any) string { return "/items/" + id },
		list: "/api/v1/items",
		cols: []importCol{{"name", "Name", tables.Text, false}, {"quantity", "Quantity", tables.Number, false}, {"unit", "Unit", tables.Text, false},
			{"location_name", "Where", tables.Select, false}, {"category_name", "Category", tables.Select, false}, {"brand", "Brand", tables.Text, false},
			{"price", "Price", tables.Currency, false}, {"expiry_date", "Use by", tables.Date, false}, {"notes", "Notes", tables.LongText, false}}},
	{slug: "place", label: "Place", plural: "places", icon: "pin", app: appPlaces, scope: scopePlacesRead,
		search: "/api/v1/search", keys: []string{"places"}, title: []string{"name"},
		sub:  func(m map[string]any) string { return firstNonEmpty(str(m, "city"), str(m, "address")) },
		open: func(id string, _ map[string]any) string { return "/places/" + id },
		list: "/api/v1/places?limit=2000",
		cols: []importCol{{"name", "Name", tables.Text, false}, {"kind", "Kind", tables.Select, false}, {"status", "Been or want", tables.Select, false},
			{"city", "City", tables.Select, false}, {"country", "Country", tables.Text, false}, {"address", "Address", tables.Text, false},
			{"rating", "Rating", tables.Rating, false}, {"website", "Website", tables.URL, false}, {"visits", "Visits", tables.Number, false}, {"last_visit", "Last visit", tables.Date, false}}},
	{slug: "deal", label: "Deal", plural: "deals", icon: "trophy", app: appCRM, scope: scopeCRMRead,
		search: "/api/v1/search", keys: []string{"deals"}, title: []string{"title"},
		sub:  func(m map[string]any) string { return firstNonEmpty(str(m, "company"), str(m, "contact_name")) },
		open: func(id string, _ map[string]any) string { return "/deals/" + id },
		list: "/api/v1/deals?limit=1000",
		cols: []importCol{{"title", "Deal", tables.Text, false}, {"stage_name", "Stage", tables.Select, false}, {"status", "Status", tables.Select, false},
			{"amount", "Value", tables.Currency, false}, {"company", "Company", tables.Text, false}, {"contact_name", "Person", tables.Text, false},
			{"contact_email", "Email", tables.Email, false}, {"next_action", "Next step", tables.Text, false}, {"next_due", "Next step on", tables.Date, false},
			{"expected_close", "Expected close", tables.Date, false}}},
	{slug: "book", label: "Book", plural: "books", icon: "journal", app: appBooks, scope: scopeBooksRead,
		search: "/api/v1/books", title: []string{"title"},
		sub:  func(m map[string]any) string { return str(m, "authors") },
		open: func(id string, _ map[string]any) string { return "/books/" + id },
		list: "/api/v1/books",
		cols: []importCol{{"title", "Title", tables.Text, false}, {"authors", "Author", tables.Text, false}, {"status", "Status", tables.Select, false},
			{"rating", "Rating", tables.Rating, false}, {"pages", "Pages", tables.Number, false}, {"published", "Published", tables.Text, false},
			{"started_on", "Started", tables.Date, false}, {"finished_on", "Finished", tables.Date, false}, {"series", "Series", tables.Text, false}}},
	{slug: "wish", label: "Wish", plural: "wishes", icon: "gift", app: appWishlist, scope: scopeWishesRead,
		search: "/api/v1/wishes", title: []string{"title"},
		sub:  func(m map[string]any) string { return firstNonEmpty(moneyText(m, "price"), str(m, "store")) },
		open: func(id string, _ map[string]any) string { return "/wishes/" + id },
		list: "/api/v1/wishes",
		cols: []importCol{{"title", "Wish", tables.Text, false}, {"list_name", "List", tables.Select, false}, {"recipient", "For", tables.Text, false},
			{"price", "Price", tables.Currency, true}, {"store", "Shop", tables.Text, false}, {"url", "Link", tables.URL, false},
			{"status", "Status", tables.Select, false}, {"priority", "Priority", tables.Number, false}}},
	{slug: "payment", label: "Payment", plural: "payments", icon: "wallet", app: appMoney, scope: scopeMoneyRead,
		search: "/api/v1/transactions", keys: []string{"items"}, title: []string{"description", "payee"},
		sub:  func(m map[string]any) string { return joinNonEmpty(" · ", dayOf(m["date"]), moneyText(m, "amount")) },
		open: func(id string, _ map[string]any) string { return "/transactions/" + id },
		list: "/api/v1/transactions?limit=1000",
		cols: []importCol{{"description", "What", tables.Text, false}, {"date", "Date", tables.Date, false}, {"amount", "Amount", tables.Currency, true},
			{"payee", "Payee", tables.Text, false}, {"category_name", "Category", tables.Select, false}, {"account_name", "Account", tables.Select, false},
			{"kind", "Kind", tables.Select, false}, {"tags", "Tags", tables.Multi, false}, {"notes", "Notes", tables.LongText, false}}},
}

func kindBySlug(slug string) (pocketKind, bool) {
	for _, k := range pocketKinds {
		if k.slug == slug {
			return k, true
		}
	}
	return pocketKind{}, false
}

// firstValue is the first of a list of strings or {value} objects.
func firstValue(v any) string {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s := firstValue(x); s != "" {
				return s
			}
		}
	case map[string]any:
		return str(t, "value", "email", "number", "address")
	case string:
		return t
	}
	return ""
}

func (k pocketKind) ref(sib *siblingConn, m map[string]any) tables.PocketRef {
	id := firstNonEmpty(str(m, "id"), idOf(m))
	return tables.PocketRef{App: k.slug, Ref: id, Title: clip(firstNonEmpty(str(m, k.title...), k.label+" "+id), 300), Sub: clip(k.sub(m), 200), URL: linkTo(sib, k.open(id, m))}
}

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

// handleKinds lists what an "In Pocket" field can point at, and whether each works.
func (s *Server) handleKinds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []map[string]any{}
	for _, k := range pocketKinds {
		st := c.status(ctx, k.app)
		out = append(out, map[string]any{"slug": k.slug, "label": k.label, "plural": k.plural, "icon": k.icon, "app": k.app.slug, "app_name": k.app.name,
			"ok": st.Via != "" && st.Can(k.scope), "state": st.State, "detail": st.Detail, "fix": st.Fix, "can_import": k.list != ""})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePick searches one kind of thing for the picker.
func (s *Server) handlePick(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	k, ok := kindBySlug(r.URL.Query().Get("kind"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown kind")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, k.app, k.scope)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "off": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	qs := url.Values{"limit": {"15"}}
	if q != "" {
		qs.Set("q", q)
	} else if k.search == "/api/v1/search" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}}) // their search needs words
		return
	}
	v, err := getJSONFrom(ctx, sib, k.search, qs)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "off": k.app.name + ": " + err.Error()})
		return
	}
	out := []tables.PocketRef{}
	for _, m := range rows(v, k.keys...) {
		out = append(out, k.ref(sib, m))
		if len(out) >= 15 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// fetchList reads a kind's whole list from its app.
func (s *Server) fetchList(ctx context.Context, uid int64, k pocketKind) ([]map[string]any, *siblingConn, error) {
	if k.list == "" {
		return nil, nil, &userErr{"a table cannot be made from " + k.plural + " yet"}
	}
	sib, err := s.clients(ctx, uid).conn(ctx, k.app, k.scope)
	if err != nil {
		return nil, nil, &userErr{err.Error()}
	}
	path, query, _ := strings.Cut(k.list, "?")
	qs, _ := url.ParseQuery(query)
	v, err := getJSONFrom(ctx, sib, path, qs)
	if err != nil {
		return nil, nil, &userErr{k.app.name + " did not answer: " + err.Error()}
	}
	return rows(v, append(k.keys, k.plural, "contacts", "tasks", "items", "results")...), sib, nil
}

// rowFrom is one thing of another app as a row: its columns and the ref.
func (k pocketKind) rowFrom(sib *siblingConn, m map[string]any) map[string]any {
	row := map[string]any{}
	for _, c := range k.cols {
		v := m[c.key]
		switch t := v.(type) {
		case []any:
			if c.typ == tables.Multi {
				var names []string
				for _, x := range t {
					if s := firstValue(x); s != "" {
						names = append(names, s)
					}
				}
				v = strings.Join(names, ", ")
			} else {
				v = firstValue(t)
			}
		case float64:
			if c.cents {
				v = math.Round(t) / 100
			}
		case map[string]any:
			v = firstValue(t)
		}
		if c.typ == tables.Date {
			v = day10(v)
		}
		if v != nil && v != "" {
			row[c.name] = v
		}
	}
	ref := k.ref(sib, m)
	row["In "+k.app.name] = []any{map[string]any{"app": ref.App, "ref": ref.Ref, "title": ref.Title, "sub": ref.Sub, "url": ref.URL}}
	return row
}

// handleFromApp makes a table from a list in another app (in a base), or
// with table set brings the new ones into a table made that way.
func (s *Server) handleFromApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Kind    string `json:"kind"`
		BaseID  int64  `json:"base_id"`
		TableID int64  `json:"table_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	k, ok := kindBySlug(in.Kind)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown kind")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	list, sib, err := s.fetchList(cctx, uid, k)
	if err != nil {
		s.userFail(w, err, "read list")
		return
	}
	if in.TableID > 0 {
		ld, err := s.loader(uid).load(ctx, in.TableID, false)
		if err != nil {
			s.fail(w, err, "table")
			return
		}
		made := 0
		for _, m := range list {
			ref := k.slug + ":" + firstNonEmpty(str(m, "id"), idOf(m))
			if _, err := s.addRecord(ctx, uid, ld, recordIn{Values: k.rowFrom(sib, m), Ref: ref, Lenient: true}, k.app.slug); err == nil {
				made++
			}
		}
		if made > 0 {
			go s.syncSheetCalendar(context.WithoutCancel(ctx), uid, ld.sheet.ID)
		}
		writeJSON(w, http.StatusOK, map[string]any{"made": made, "seen": len(list)})
		return
	}
	spec := specTable{Name: strings.ToUpper(k.plural[:1]) + k.plural[1:], Icon: "", Description: "Made from " + k.app.name + " on " + today() + "."}
	for _, c := range k.cols {
		spec.Fields = append(spec.Fields, specField{Name: c.name, Type: c.typ})
	}
	spec.Fields = append(spec.Fields, specField{Name: "In " + k.app.name, Type: tables.Pocket, App: k.slug, Options: tables.Options{Single: true}})
	base := specBase{Name: k.app.name, Icon: "📥", Tables: []specTable{spec}}
	b, err := s.build(ctx, uid, base, in.BaseID, false)
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
	ld, err := s.loader(uid).load(ctx, t.ID, false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	made := 0
	for _, m := range list {
		ref := k.slug + ":" + firstNonEmpty(str(m, "id"), idOf(m))
		if _, err := s.addRecord(ctx, uid, ld, recordIn{Values: k.rowFrom(sib, m), Ref: ref, Lenient: true}, k.app.slug); err == nil {
			made++
		}
	}
	t, _ = s.store.GetSheet(ctx, uid, t.ID)
	t.Settings.FromApp = k.slug
	_ = s.store.UpdateSheet(ctx, t)
	writeJSON(w, http.StatusCreated, map[string]any{"table": t, "made": made})
}
