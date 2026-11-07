package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- a row in the other apps ----
//
// From one row, as its owner: a task in Pocket Tasks, a note in Pocket
// Notes, an event in Pocket Calendar. A table can also keep its dates in
// the calendar by itself, take the answers to a form as new rows, and be
// saved as a spreadsheet in Pocket Files. Nothing here is needed for the
// table itself: an app that is away only greys out its own button.

// recordMarkdown is a row as Markdown: its fields as a list.
func (s *Server) recordMarkdown(ld *loaded, x *tables.Record) string {
	var b strings.Builder
	for _, f := range ld.fields {
		if f.Primary {
			continue
		}
		t := ld.eng.Text(x, f)
		if t == "" {
			continue
		}
		if f.Type == tables.LongText {
			fmt.Fprintf(&b, "**%s**\n\n%s\n\n", f.Name, t)
			continue
		}
		fmt.Fprintf(&b, "- **%s:** %s\n", f.Name, strings.ReplaceAll(t, "\n", " "))
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) origin(ld *loaded, id int64) string {
	link := s.recordLink(id)
	if link == "" {
		return "From Pocket Tables — " + ld.sheet.Name
	}
	return fmt.Sprintf("From Pocket Tables — %s: %s", ld.sheet.Name, link)
}

type makeIn struct {
	Target  string `json:"target"` // task | note | event
	Title   string `json:"title"`
	Due     string `json:"due"`
	Date    string `json:"date"`
	Time    string `json:"time"`
	Minutes int    `json:"minutes"`
	Notes   string `json:"notes"`
}

func plainErr(a pocketApp, err error) string {
	if isNotConnected(err) {
		return err.Error()
	}
	if errors.Is(err, pocket.ErrNotFound) {
		return a.name + " does not know this (is it up to date?)"
	}
	return a.name + " did not answer: " + err.Error()
}

// handleMake makes a task, note or event from a row.
func (s *Server) handleMake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
		return
	}
	var in makeIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ld, err := s.loader(uid).load(ctx, rec.SheetID, false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	x := ld.eng.Record(rec.ID)
	if x == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	c := s.clients(ctx, uid)
	title := clip(firstNonEmpty(strings.TrimSpace(in.Title), titleOr(ld.eng.Title(x), x.Number)), 200)
	var (
		a    pocketApp
		res  map[string]any
		path string
	)
	switch in.Target {
	case "task":
		a = appTasks
		due := firstNonEmpty(in.Due, in.Date)
		if due == "" && ld.sheet.Settings.TaskDue > 0 {
			due = clip(tables.ToText(x.Values[tables.Key(ld.sheet.Settings.TaskDue)]), 10)
		}
		if due != "" && !validDate(due) {
			writeErr(w, http.StatusBadRequest, "the due day is YYYY-MM-DD")
			return
		}
		var sib *siblingConn
		if sib, err = c.conn(ctx, a, scopeTasksWrite); err == nil {
			body := map[string]any{"title": title, "notes": clip(joinNonEmpty("\n\n", in.Notes, s.recordMarkdown(ld, x), s.origin(ld, x.ID)), 20000), "url": s.recordLink(x.ID)}
			if due != "" {
				body["due"] = due
			}
			if res, err = postJSONTo(ctx, sib, "/api/v1/tasks", body); err == nil {
				path = "/inbox?task=" + idOf(res)
			}
		}
	case "note":
		a = appNotes
		var sib *siblingConn
		if sib, err = c.conn(ctx, a, scopeNotesWrite); err == nil {
			content := joinNonEmpty("\n\n", in.Notes, s.recordMarkdown(ld, x), "— "+s.origin(ld, x.ID))
			if res, err = postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": title, "content": content}); err == nil {
				path = "/note/" + idOf(res)
			}
		}
	case "event":
		a = appCalendar
		date := firstNonEmpty(in.Date, in.Due)
		if !validDate(date) || date == "" {
			writeErr(w, http.StatusBadRequest, "pick the day")
			return
		}
		body := map[string]any{"title": title, "description": joinNonEmpty("\n\n", in.Notes, s.recordMarkdown(ld, x), s.origin(ld, x.ID)), "tz": time.Local.String(), "url": s.recordLink(x.ID)}
		if in.Time == "" {
			body["start"], body["all_day"] = date, true
		} else {
			st, perr := time.ParseInLocation("2006-01-02 15:04", date+" "+in.Time, time.Local)
			if perr != nil {
				writeErr(w, http.StatusBadRequest, "the time is HH:MM")
				return
			}
			mins := in.Minutes
			if mins <= 0 {
				mins = max(c.st.MeetingMinutes, 15)
			}
			body["start"], body["end"] = st.Format("2006-01-02T15:04"), st.Add(time.Duration(mins)*time.Minute).Format("2006-01-02T15:04")
		}
		var sib *siblingConn
		if sib, err = c.conn(ctx, a, scopeCalendarWrite); err == nil {
			if res, err = postJSONTo(ctx, sib, "/api/v1/events", body); err == nil {
				path = "/event/" + firstNonEmpty(str(res, "id"), idOf(res))
			}
		}
	default:
		writeErr(w, http.StatusBadRequest, "make a task, a note or an event")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(a, err))
		return
	}
	m := &store.Made{RecordID: x.ID, Target: in.Target, App: a.slug, Ref: firstNonEmpty(str(res, "event_id"), idOf(res)), Title: title, State: "open"}
	if sib, _ := c.conn(ctx, a); sib != nil {
		m.URL = linkTo(sib, path)
	}
	if m.URL == "" {
		m.URL = s.siblingLink(ctx, uid, a, path)
	}
	if err := s.store.AddMade(ctx, uid, m); err != nil {
		s.fail(w, err, "remember")
		return
	}
	word := map[string]string{"task": "A task", "note": "A note", "event": "An event"}[in.Target]
	_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: x.ID, Kind: "made", Text: word + " in " + a.name + ": " + title, Data: map[string]any{"url": m.URL}, App: a.slug})
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleDeleteMade(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteMade(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "forget")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tableMarkdown is a view of a table as a Markdown table (at most 500 rows).
func (s *Server) tableMarkdown(ld *loaded, v *store.View, now time.Time) string {
	fields := s.visibleFields(ld, v)
	if len(fields) > 12 {
		fields = fields[:12]
	}
	esc := strings.NewReplacer("|", "\\|", "\n", " ")
	var b strings.Builder
	b.WriteString("|")
	for _, f := range fields {
		b.WriteString(" " + esc.Replace(f.Name) + " |")
	}
	b.WriteString("\n|")
	for range fields {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	recs := ld.view(v, "", now)
	for i, x := range recs {
		if i >= 500 {
			fmt.Fprintf(&b, "\n…and %d more rows.\n", len(recs)-500)
			break
		}
		b.WriteString("|")
		for _, f := range fields {
			b.WriteString(" " + esc.Replace(clip(ld.eng.Text(x, f), 300)) + " |")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// handleTableNote saves a table (as a view shows it) as a note in Pocket Notes.
func (s *Server) handleTableNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l := s.loader(uid)
	ld, err := l.load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	v := s.viewOf(ctx, uid, ld, r)
	sib, err := s.clients(ctx, uid).conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appNotes, err))
		return
	}
	title := ld.sheet.Name
	if v != nil && len(v.Config.Filters) > 0 {
		title += " — " + v.Name
	}
	content := joinNonEmpty("\n\n", ld.sheet.Description, s.tableMarkdown(ld, v, l.now), "— From Pocket Tables, "+today()+": "+s.tableLink(ld.sheet.ID))
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(title, 200), "content": content})
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appNotes, err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": linkTo(sib, "/note/"+idOf(res))})
}

// ---- Pocket Files ----

// handleToFiles saves a table as a CSV file in the person's Pocket Files (folder "Tables").
func (s *Server) handleToFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l := s.loader(uid)
	ld, err := l.load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	v := s.viewOf(ctx, uid, ld, r)
	sib, err := s.clients(ctx, uid).conn(ctx, appFiles, scopeFilesWrite)
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appFiles, err))
		return
	}
	name := safeName(ld.sheet.Name) + " " + today() + ".csv"
	var out map[string]any
	q := url.Values{"space": {"home"}, "path": {"/Tables"}, "name": {name}, "conflict": {"overwrite"}}
	if err := sib.client.PutRaw(ctx, "/api/v1/fs/upload", q, s.tableCSV(ld, v, l.now), "text/csv", &out); err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appFiles, err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "url": linkTo(sib, "/f/home/Tables?open="+url.QueryEscape(name))})
}

// handleFilesList lists a folder of Pocket Files: folders and spreadsheets.
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appFiles, scopeFilesRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "off": err.Error()})
		return
	}
	space := firstNonEmpty(r.URL.Query().Get("space"), "home")
	path := firstNonEmpty(r.URL.Query().Get("path"), "/")
	v, err := getJSONFrom(ctx, sib, "/api/v1/fs/list", url.Values{"space": {space}, "path": {path}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}, "off": plainErr(appFiles, err)})
		return
	}
	out := []map[string]any{}
	for _, e := range rows(v, "entries") {
		name := str(e, "name")
		dir := e["is_dir"] == true
		ext := strings.ToLower(name[strings.LastIndex(name, ".")+1:])
		if !dir && !oneOf(ext, []string{"csv", "tsv", "txt"}) {
			continue
		}
		out = append(out, map[string]any{"name": name, "path": str(e, "path"), "dir": dir, "size": num(e, "size"), "modified": str(e, "modified")})
	}
	spaces := []map[string]any{}
	if sv, err := getJSONFrom(ctx, sib, "/api/v1/spaces", nil); err == nil {
		for _, sp := range rows(sv, "spaces") {
			spaces = append(spaces, map[string]any{"id": str(sp, "id"), "name": str(sp, "name")})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out, "space": space, "path": path, "spaces": spaces})
}

// handleFromFiles makes a table in a base from a spreadsheet in Pocket Files.
func (s *Server) handleFromFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.store.GetBase(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "base")
		return
	}
	var in struct {
		Space string `json:"space"`
		Path  string `json:"path"`
	}
	if err := decode(r, &in); err != nil || in.Path == "" {
		writeErr(w, http.StatusBadRequest, "pick a file")
		return
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appFiles, scopeFilesRead)
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appFiles, err))
		return
	}
	q := url.Values{"space": {firstNonEmpty(in.Space, "home")}, "path": {in.Path}}
	data, _, err := sib.client.Bytes(ctx, "/api/v1/fs/raw?"+q.Encode(), 50<<20)
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appFiles, err))
		return
	}
	grid, err := parseSheet(data)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	name := in.Path[strings.LastIndex(in.Path, "/")+1:]
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	t, n, err := s.importGrid(ctx, uid, b.ID, name, grid)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"table": t, "made": n})
}

// ---- keeping a table's dates in Pocket Calendar ----

func stampOf(v any) string {
	h := sha1.Sum([]byte(fmt.Sprint(v)))
	return hex.EncodeToString(h[:8])
}

// eventBody is the calendar event of a row ("" when it has no date).
func (s *Server) eventBody(ld *loaded, x *tables.Record, c *store.CalendarSync, minutes int) map[string]any {
	f := ld.fieldByID(c.Field)
	if f == nil {
		return nil
	}
	start := tables.ToText(x.Values[tables.Key(f.ID)])
	if start == "" {
		return nil
	}
	body := map[string]any{"title": titleOr(ld.eng.Title(x), x.Number), "description": joinNonEmpty("\n\n", s.recordMarkdown(ld, x), s.origin(ld, x.ID)),
		"tz": time.Local.String(), "url": s.recordLink(x.ID)}
	end := ""
	if c.EndField > 0 {
		end = tables.ToText(x.Values[tables.Key(c.EndField)])
	}
	if len(start) == 10 {
		body["start"], body["all_day"] = start, true
		if len(end) >= 10 && end[:10] >= start {
			e, _ := time.Parse("2006-01-02", end[:10])
			body["end"] = e.AddDate(0, 0, 1).Format("2006-01-02")
		}
		return body
	}
	st, err := time.ParseInLocation("2006-01-02T15:04", clip(start, 16), time.Local)
	if err != nil {
		return nil
	}
	body["start"] = st.Format("2006-01-02T15:04")
	if et, err := time.ParseInLocation("2006-01-02T15:04", clip(end, 16), time.Local); err == nil && et.After(st) {
		body["end"] = et.Format("2006-01-02T15:04")
	} else {
		body["end"] = st.Add(time.Duration(max(minutes, 15)) * time.Minute).Format("2006-01-02T15:04")
	}
	return body
}

// syncRecordCalendar puts one row's event in the calendar, changes it, or takes it away.
func (s *Server) syncRecordCalendar(ctx context.Context, uid int64, ld *loaded, id int64) {
	c := ld.sheet.Settings.Calendar
	x := ld.eng.Record(id)
	var body map[string]any
	if c != nil && c.On && x != nil {
		body = s.eventBody(ld, x, c, s.settingsOf(ctx, uid).MeetingMinutes)
	}
	made, _ := s.store.MadeFor(ctx, uid, id, "sync")
	if body == nil {
		for _, m := range made {
			s.dropEvent(ctx, uid, m)
		}
		return
	}
	sib, err := optional(s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite))
	if err != nil || sib == nil {
		return
	}
	stamp := stampOf(body)
	if len(made) > 0 {
		m := made[0]
		if m.Stamp == stamp {
			return
		}
		var out map[string]any
		err := sib.client.Do(ctx, http.MethodPut, "/api/v1/events/"+url.PathEscape(m.Ref), url.Values{"scope": {"all"}}, body, &out)
		if err == nil {
			m.Stamp, m.Title, m.State = stamp, tables.ToText(body["title"]), "synced"
			_ = s.store.UpdateMade(ctx, uid, m)
			return
		}
		if !errors.Is(err, pocket.ErrNotFound) {
			s.log.Debug("calendar: update", "row", id, "err", err)
			return
		}
		_ = s.store.DeleteMade(ctx, uid, m.ID) // gone there: make it again
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		s.log.Debug("calendar: add", "row", id, "err", err)
		return
	}
	ref := firstNonEmpty(fmt.Sprint(int64(num(res, "event_id"))), idOf(res))
	if ref == "0" {
		ref = idOf(res)
	}
	_ = s.store.AddMade(ctx, uid, &store.Made{RecordID: id, Target: "sync", App: appCalendar.slug, Ref: ref, Title: tables.ToText(body["title"]),
		URL: linkTo(sib, "/event/"+firstNonEmpty(str(res, "id"), ref)), State: "synced", Stamp: stamp})
}

func (s *Server) dropEvent(ctx context.Context, uid int64, m *store.Made) {
	if sib, err := optional(s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite)); err == nil && sib != nil && m.Ref != "" {
		if err := sib.client.Do(ctx, http.MethodDelete, "/api/v1/events/"+url.PathEscape(m.Ref), url.Values{"scope": {"all"}}, nil, nil); err != nil && !errors.Is(err, pocket.ErrNotFound) {
			s.log.Debug("calendar: remove", "event", m.Ref, "err", err)
			return
		}
	}
	_ = s.store.DeleteMade(ctx, uid, m.ID)
}

// syncSheetCalendar brings a whole table's events up to date (and takes
// away the ones of deleted rows, or all when the sync was switched off).
func (s *Server) syncSheetCalendar(ctx context.Context, uid, sheetID int64) {
	ld, err := s.loader(uid).load(ctx, sheetID, false)
	if err != nil {
		return
	}
	made, _ := s.store.MadeInSheet(ctx, uid, sheetID, "sync")
	for _, m := range made {
		if ld.byID[m.RecordID] == nil {
			s.dropEvent(ctx, uid, m)
		}
	}
	c := ld.sheet.Settings.Calendar
	if c == nil {
		return
	}
	for _, x := range ld.eng.Records {
		s.syncRecordCalendar(ctx, uid, ld, x.ID)
	}
}

// unsyncRecord takes a row's events off the calendar.
func (s *Server) unsyncRecord(ctx context.Context, uid, id int64) {
	made, _ := s.store.MadeFor(ctx, uid, id, "sync")
	for _, m := range made {
		s.dropEvent(ctx, uid, m)
	}
}

// unsyncSheet takes all of a table's events off the calendar.
func (s *Server) unsyncSheet(ctx context.Context, uid, sheetID int64) {
	made, _ := s.store.MadeInSheet(ctx, uid, sheetID, "sync")
	for _, m := range made {
		s.dropEvent(ctx, uid, m)
	}
}

// ---- answers to a form as rows ----

var formTypes = map[string]string{"text": tables.Text, "long": tables.LongText, "email": tables.Email, "phone": tables.Phone, "number": tables.Number,
	"url": tables.URL, "date": tables.Date, "time": tables.Text, "choice": tables.Select, "dropdown": tables.Select, "checkboxes": tables.Multi,
	"yesno": tables.Checkbox, "rating": tables.Rating, "consent": tables.Checkbox}

// handleForms lists the person's forms in Pocket Forms with their questions.
func (s *Server) handleForms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appForms, scopeFormsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"forms": []any{}, "off": err.Error()})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/forms", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"forms": []any{}, "off": plainErr(appForms, err)})
		return
	}
	out := []map[string]any{}
	for _, f := range rows(v, "forms") {
		var qs []map[string]any
		for _, q := range rows(f["fields"]) {
			if str(q, "type") == "section" {
				continue
			}
			qs = append(qs, map[string]any{"id": str(q, "id"), "label": str(q, "label"), "type": str(q, "type"), "options": q["options"], "suggest": formTypes[str(q, "type")]})
		}
		out = append(out, map[string]any{"id": idOf(f), "title": str(f, "title"), "emoji": str(f, "emoji"), "status": str(f, "status"), "responses": num(f, "responses"), "fields": qs,
			"url": linkTo(sib, "/forms/"+idOf(f))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"forms": out})
}

// handleSetForm ties a form to a table: its answers become rows. With
// make_fields the questions without a field get one.
func (s *Server) handleSetForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ld, err := s.loader(uid).load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in struct {
		On         bool             `json:"on"`
		FormID     string           `json:"form_id"`
		Title      string           `json:"title"`
		Map        map[string]int64 `json:"map"`
		MakeFields bool             `json:"make_fields"`
		Questions  []struct {
			ID      string   `json:"id"`
			Label   string   `json:"label"`
			Type    string   `json:"type"`
			Options []string `json:"options"`
		} `json:"questions"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t := ld.sheet
	if !in.On || in.FormID == "" {
		if t.Settings.Form != nil {
			t.Settings.Form.On = false
		}
		_ = s.store.UpdateSheet(ctx, t)
		p, _ := s.sheetPage(ctx, uid, t.ID)
		writeJSON(w, http.StatusOK, p)
		return
	}
	m := map[string]int64{}
	for q, fid := range in.Map {
		if f := ld.fieldByID(fid); f != nil && !tables.Computed(f.Type) && f.Type != tables.Link {
			m[q] = fid
		}
	}
	if in.MakeFields {
		for _, q := range in.Questions {
			if _, ok := m[q.ID]; ok || q.Label == "" {
				continue
			}
			typ := formTypes[q.Type]
			if typ == "" {
				typ = tables.Text
			}
			f := &tables.Field{TableID: t.ID, Name: freeName(ld, clip(strings.NewReplacer("{", "(", "}", ")").Replace(q.Label), 100)), Type: typ}
			for _, o := range q.Options {
				f.Options.Choices = append(f.Options.Choices, tables.Choice{Name: o})
			}
			if s.cleanField(ctx, uid, ld, f) != nil || s.store.CreateField(ctx, uid, f) != nil {
				continue
			}
			ld.fields = append(ld.fields, f)
			m[q.ID] = f.ID
		}
	}
	prev := t.Settings.Form
	t.Settings.Form = &store.FormSync{On: true, FormID: in.FormID, Title: clip(in.Title, 200), Map: m}
	if prev != nil && prev.FormID == in.FormID {
		t.Settings.Form.Made = prev.Made
	}
	if err := s.store.UpdateSheet(ctx, t); err != nil {
		s.fail(w, err, "save")
		return
	}
	s.pullForm(ctx, uid, t.ID)
	p, err := s.sheetPage(ctx, uid, t.ID)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePullForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := s.store.GetSheet(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	n := s.pullForm(ctx, uid, t.ID)
	t, _ = s.store.GetSheet(ctx, uid, t.ID)
	writeJSON(w, http.StatusOK, map[string]any{"made": n, "form": t.Settings.Form})
}

// pullForm adds the answers not yet in the table as rows and says how many.
func (s *Server) pullForm(ctx context.Context, uid, sheetID int64) int {
	t, err := s.store.GetSheet(ctx, uid, sheetID)
	if err != nil || t.Settings.Form == nil || !t.Settings.Form.On {
		return 0
	}
	fs := t.Settings.Form
	save := func(status string) {
		fs.Status, fs.At = status, store.Now()
		t.Settings.Form = fs
		_ = s.store.UpdateSheet(ctx, t)
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appForms, scopeFormsRead)
	if err != nil {
		save(err.Error())
		return 0
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/responses", url.Values{"form": {fs.FormID}, "limit": {"500"}})
	if err != nil {
		save(plainErr(appForms, err))
		return 0
	}
	ld, err := s.loader(uid).load(ctx, sheetID, false)
	if err != nil {
		return 0
	}
	made := 0
	for _, resp := range rows(v, "items") {
		if str(resp, "status") == "spam" {
			continue
		}
		ref := "answer:" + idOf(resp)
		if _, err := s.store.RecordBySource(ctx, uid, sheetID, ref); err == nil {
			continue
		}
		one, err := getJSONFrom(ctx, sib, "/api/v1/responses/"+idOf(resp), nil)
		if err != nil {
			continue
		}
		answers := obj(obj(one.(map[string]any), "response"), "answers")
		vals := map[string]any{}
		for q, fid := range fs.Map {
			if a, ok := answers[q]; ok && a != nil {
				vals[tables.Key(fid)] = a
			}
		}
		if len(vals) == 0 {
			continue
		}
		rec, err := s.addRecord(ctx, uid, ld, recordIn{Values: vals, Ref: ref, Lenient: true}, appForms.slug)
		if err != nil {
			continue
		}
		if c := str(resp, "created_at"); c != "" {
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE records SET created_at = ? WHERE id = ?`, c, rec.ID)
		}
		made++
		s.afterWrite(ctx, uid, sheetID, rec.ID, "tables.record_created", nil)
	}
	fs.Made += made
	save("ok")
	return made
}

// handleMakeForm makes a draft form in Pocket Forms with a table's fields as
// questions and ties it to the table, so its answers come back as rows.
func (s *Server) handleMakeForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ld, err := s.loader(uid).load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appForms, scopeFormsWrite)
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appForms, err))
		return
	}
	var qs []map[string]any
	m := map[string]int64{}
	for _, f := range ld.fields {
		typ := map[string]string{tables.Text: "text", tables.LongText: "long", tables.Number: "number", tables.Currency: "number", tables.Percent: "number",
			tables.Rating: "rating", tables.Checkbox: "yesno", tables.Date: "date", tables.Select: "dropdown", tables.Multi: "checkboxes",
			tables.Email: "email", tables.Phone: "phone", tables.URL: "url"}[f.Type]
		if typ == "" {
			continue
		}
		id := "f" + itoa(f.ID)
		q := map[string]any{"id": id, "type": typ, "label": f.Name, "required": f.Primary || f.Required}
		if f.Description != "" {
			q["help"] = f.Description
		}
		if typ == "dropdown" && len(f.Options.Choices) <= 6 {
			q["type"] = "choice"
		}
		var opts []string
		for _, c := range f.Options.Choices {
			opts = append(opts, c.Name)
		}
		if len(opts) > 0 {
			q["options"] = opts
		}
		switch f.Type {
		case tables.Email:
			q["role"] = "email"
		}
		qs = append(qs, q)
		m[id] = f.ID
	}
	if len(qs) == 0 {
		writeErr(w, http.StatusBadRequest, "the table has no fields a form can ask for")
		return
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/forms", map[string]any{"title": ld.sheet.Name, "description": ld.sheet.Description, "fields": qs})
	if err != nil {
		writeErr(w, http.StatusBadGateway, plainErr(appForms, err))
		return
	}
	// Forms may have given the questions other ids
	if back := rows(res["fields"]); len(back) == len(qs) {
		nm := map[string]int64{}
		for i, q := range back {
			nm[str(q, "id")] = m[tables.ToText(qs[i]["id"])]
		}
		m = nm
	}
	t := ld.sheet
	t.Settings.Form = &store.FormSync{On: true, FormID: idOf(res), Title: str(res, "title"), Map: m, URL: linkTo(sib, "/forms/"+idOf(res)), Status: "ok", At: store.Now()}
	if err := s.store.UpdateSheet(ctx, t); err != nil {
		s.fail(w, err, "save")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"url": t.Settings.Form.URL, "form": t.Settings.Form})
}

// ---- looking at the other apps now and then ----

// syncAll runs every ten minutes: new answers to tied forms become rows, and
// tasks made from rows that were ticked off in Pocket Tasks tick the row.
func (s *Server) syncAll(ctx context.Context) {
	if !s.syncMu.TryLock() {
		return
	}
	defer s.syncMu.Unlock()
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		sheets, _ := s.store.ListSheets(ctx, uid, 0)
		for _, t := range sheets {
			if t.Settings.Form != nil && t.Settings.Form.On {
				s.pullForm(ctx, uid, t.ID)
			}
		}
		s.checkTasks(ctx, uid)
	}
}

func (s *Server) checkTasks(ctx context.Context, uid int64) {
	open, err := s.store.OpenTasks(ctx, uid)
	if err != nil || len(open) == 0 {
		return
	}
	sib, err := optional(s.clients(ctx, uid).conn(ctx, appTasks, scopeTasksRead))
	if err != nil || sib == nil {
		return
	}
	for _, m := range open {
		v, err := getJSONFrom(ctx, sib, "/api/v1/tasks/"+url.PathEscape(m.Ref), nil)
		if err != nil {
			if errors.Is(err, pocket.ErrNotFound) {
				m.State = "gone"
				_ = s.store.UpdateMade(ctx, uid, m)
			}
			continue
		}
		t, _ := v.(map[string]any)
		if inner := obj(t, "task"); inner != nil {
			t = inner
		}
		if str(t, "status") != "done" {
			_ = s.store.UpdateMade(ctx, uid, m)
			continue
		}
		m.State = "done"
		_ = s.store.UpdateMade(ctx, uid, m)
		rec, err := s.store.GetRecord(ctx, uid, m.RecordID)
		if err != nil || rec.DeletedAt != "" {
			continue
		}
		_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: rec.ID, Kind: "made", Text: "Task done in Pocket Tasks: " + m.Title, App: "tasks"})
		sh, err := s.store.GetSheet(ctx, uid, rec.SheetID)
		if err != nil || sh.Settings.TaskDone == 0 {
			continue
		}
		k := tables.Key(sh.Settings.TaskDone)
		if tables.ToBool(rec.Data[k]) {
			continue
		}
		rec.Data[k] = true
		if s.store.SaveRecord(ctx, rec) == nil {
			s.afterWrite(ctx, uid, rec.SheetID, rec.ID, "tables.record_updated", []change{{Field: "done", Before: "", After: "yes"}})
		}
	}
}
