package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- bases ----

// BaseColors are offered for a base.
var BaseColors = []string{"#087f5b", "#1c7ed6", "#7048e8", "#c2255c", "#e8590c", "#f08c00", "#0c8599", "#5c940d", "#495057"}

// tree is every base with its tables.
func (s *Server) tree(ctx context.Context, uid int64) ([]*store.Base, error) {
	bases, err := s.store.ListBases(ctx, uid)
	if err != nil {
		return nil, err
	}
	sheets, err := s.store.ListSheets(ctx, uid, 0)
	if err != nil {
		return nil, err
	}
	for _, b := range bases {
		b.Tables = []*store.Sheet{}
		for _, t := range sheets {
			if t.BaseID == b.ID {
				b.Tables = append(b.Tables, t)
			}
		}
	}
	return bases, nil
}

func (s *Server) handleListBases(w http.ResponseWriter, r *http.Request) {
	bases, err := s.tree(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "bases")
		return
	}
	writeJSON(w, http.StatusOK, bases)
}

type baseIn struct {
	Name        *string `json:"name"`
	Icon        *string `json:"icon"`
	Color       *string `json:"color"`
	Description *string `json:"description"`
	Empty       bool    `json:"empty"` // no first table
}

func (in baseIn) apply(b *store.Base) error {
	if in.Name != nil {
		b.Name = clip(strings.TrimSpace(*in.Name), 100)
	}
	if b.Name == "" {
		return &userErr{"give the base a name"}
	}
	if in.Icon != nil {
		b.Icon = clip(strings.TrimSpace(*in.Icon), 16)
	}
	if in.Color != nil {
		c := strings.TrimSpace(*in.Color)
		if c != "" && !reHex.MatchString(c) {
			return &userErr{"the colour is #rrggbb"}
		}
		b.Color = c
	}
	if in.Description != nil {
		b.Description = clip(strings.TrimSpace(*in.Description), 2000)
	}
	return nil
}

func (s *Server) handleCreateBase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in baseIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	b := &store.Base{UserID: uid, Icon: "📋"}
	if err := in.apply(b); err != nil {
		s.userFail(w, err, "base")
		return
	}
	if b.Color == "" {
		n, _ := s.store.ListBases(ctx, uid)
		b.Color = BaseColors[len(n)%len(BaseColors)]
	}
	if err := s.store.CreateBase(ctx, b); err != nil {
		s.fail(w, err, "create base")
		return
	}
	b.Tables = []*store.Sheet{}
	if !in.Empty {
		t, err := s.newSheet(ctx, uid, b.ID, "Table 1", "", nil)
		if err != nil {
			s.fail(w, err, "create table")
			return
		}
		b.Tables = append(b.Tables, t)
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleUpdateBase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.store.GetBase(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "base")
		return
	}
	var in baseIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(b); err != nil {
		s.userFail(w, err, "base")
		return
	}
	if err := s.store.UpdateBase(ctx, b); err != nil {
		s.fail(w, err, "save base")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleDeleteBase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetBase(ctx, uid, id); err != nil {
		s.fail(w, err, "base")
		return
	}
	sheets, _ := s.store.ListSheets(ctx, uid, id)
	for _, t := range sheets {
		s.unsyncSheet(ctx, uid, t.ID)
	}
	if err := s.store.DeleteBase(ctx, uid, id); err != nil {
		s.fail(w, err, "delete base")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleOrder puts bases, tables, fields or views in the order of the ids sent.
func (s *Server) handleOrder(what string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		uid := actorID(ctx)
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		table := map[string]string{"bases": "bases", "tables": "sheets", "fields": "fields", "views": "views"}[what]
		for i, id := range in.IDs {
			if _, err := s.store.DB.ExecContext(ctx, `UPDATE `+table+` SET position = ? WHERE user_id = ? AND id = ?`, float64(i+1), uid, id); err != nil {
				s.fail(w, err, "order")
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- tables ----

// newSheet makes a table with the fields a new table starts with (or the
// given ones) and a grid view.
func (s *Server) newSheet(ctx context.Context, uid, baseID int64, name, icon string, fields []*tables.Field) (*store.Sheet, error) {
	t := &store.Sheet{UserID: uid, BaseID: baseID, Name: name, Icon: icon}
	if err := s.store.CreateSheet(ctx, t); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = []*tables.Field{
			{Name: "Name", Type: tables.Text, Primary: true},
			{Name: "Notes", Type: tables.LongText},
			{Name: "Status", Type: tables.Select, Options: tables.Options{Choices: tables.CleanChoices([]tables.Choice{{Name: "To do", Color: "#ced4da"}, {Name: "Doing", Color: "#74c0fc"}, {Name: "Done", Color: "#69db7c"}})}},
		}
	}
	for _, f := range fields {
		f.TableID = t.ID
		if err := s.store.CreateField(ctx, uid, f); err != nil {
			return nil, err
		}
	}
	v := &store.View{UserID: uid, SheetID: t.ID, Name: "Grid", Type: "grid"}
	if err := s.store.CreateView(ctx, v); err != nil {
		return nil, err
	}
	return t, nil
}

// sheetPage is a table with what its page needs.
type sheetPage struct {
	Table   *store.Sheet    `json:"table"`
	Base    *store.Base     `json:"base"`
	Fields  []*tables.Field `json:"fields"`
	Views   []*store.View   `json:"views"`
	Others  []*store.Sheet  `json:"others"` // every table of the person, for links
	Deleted int             `json:"deleted"`
}

func (s *Server) sheetPage(ctx context.Context, uid, id int64) (*sheetPage, error) {
	t, err := s.store.GetSheet(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	b, err := s.store.GetBase(ctx, uid, t.BaseID)
	if err != nil {
		return nil, err
	}
	fields, err := s.store.Fields(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	views, err := s.store.Views(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	others, err := s.store.ListSheets(ctx, uid, 0)
	if err != nil {
		return nil, err
	}
	p := &sheetPage{Table: t, Base: b, Fields: fields, Views: views, Others: others}
	_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM records WHERE sheet_id = ? AND deleted_at IS NOT NULL`, id).Scan(&p.Deleted)
	return p, nil
}

func (s *Server) handleGetSheet(w http.ResponseWriter, r *http.Request) {
	p, err := s.sheetPage(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreateSheet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.store.GetBase(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "base")
		return
	}
	var in struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := clip(strings.TrimSpace(in.Name), 100)
	if name == "" {
		sheets, _ := s.store.ListSheets(ctx, uid, b.ID)
		name = "Table " + itoa(int64(len(sheets)+1))
	}
	t, err := s.newSheet(ctx, uid, b.ID, name, clip(in.Icon, 16), nil)
	if err != nil {
		s.fail(w, err, "create table")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

type sheetIn struct {
	Name        *string             `json:"name"`
	Icon        *string             `json:"icon"`
	Description *string             `json:"description"`
	BaseID      *int64              `json:"base_id"`
	Calendar    *store.CalendarSync `json:"calendar"`
	TaskDone    *int64              `json:"task_done"`
	TaskDue     *int64              `json:"task_due"`
}

func (s *Server) handleUpdateSheet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := s.store.GetSheet(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in sheetIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil {
		if n := clip(strings.TrimSpace(*in.Name), 100); n != "" {
			t.Name = n
		}
	}
	if in.Icon != nil {
		t.Icon = clip(strings.TrimSpace(*in.Icon), 16)
	}
	if in.Description != nil {
		t.Description = clip(strings.TrimSpace(*in.Description), 4000)
	}
	if in.BaseID != nil && *in.BaseID != t.BaseID {
		if _, err := s.store.GetBase(ctx, uid, *in.BaseID); err != nil {
			writeErr(w, http.StatusBadRequest, "there is no such base")
			return
		}
		t.BaseID, t.Position = *in.BaseID, 0
		_ = s.store.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM sheets WHERE user_id = ? AND base_id = ?`, uid, t.BaseID).Scan(&t.Position)
	}
	fields, _ := s.store.Fields(ctx, uid, t.ID)
	typed := func(id int64, types ...string) bool {
		for _, f := range fields {
			if f.ID == id {
				return oneOf(f.Type, types)
			}
		}
		return false
	}
	resync := false
	if in.Calendar != nil {
		c := *in.Calendar
		if c.On && !typed(c.Field, tables.Date) {
			writeErr(w, http.StatusBadRequest, "pick a date field to keep in Pocket Calendar")
			return
		}
		if c.EndField != 0 && !typed(c.EndField, tables.Date) {
			c.EndField = 0
		}
		was := t.Settings.Calendar
		t.Settings.Calendar = &c
		resync = c.On || (was != nil && was.On)
	}
	if in.TaskDone != nil {
		if *in.TaskDone != 0 && !typed(*in.TaskDone, tables.Checkbox) {
			writeErr(w, http.StatusBadRequest, "pick a checkbox field")
			return
		}
		t.Settings.TaskDone = *in.TaskDone
	}
	if in.TaskDue != nil {
		if *in.TaskDue != 0 && !typed(*in.TaskDue, tables.Date) {
			writeErr(w, http.StatusBadRequest, "pick a date field")
			return
		}
		t.Settings.TaskDue = *in.TaskDue
	}
	if err := s.store.UpdateSheet(ctx, t); err != nil {
		s.fail(w, err, "save table")
		return
	}
	if resync {
		go s.syncSheetCalendar(context.WithoutCancel(ctx), uid, t.ID)
	}
	p, err := s.sheetPage(ctx, uid, t.ID)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteSheet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetSheet(ctx, uid, id); err != nil {
		s.fail(w, err, "table")
		return
	}
	s.unsyncSheet(ctx, uid, id)
	if err := s.store.DeleteSheet(ctx, uid, id); err != nil {
		s.fail(w, err, "delete table")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDuplicateSheet copies a table: its fields and views, and with rows set its rows.
func (s *Server) handleDuplicateSheet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Rows bool `json:"rows"`
	}
	_ = decode(r, &in)
	ld, err := s.loader(uid).load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var fields []*tables.Field
	idMap := map[int64]*tables.Field{}
	for _, f := range ld.fields {
		if f.Type == tables.Link || f.Type == tables.Lookup || f.Type == tables.Rollup {
			continue // links belong to the original
		}
		c := *f
		c.ID = 0
		fields = append(fields, &c)
		idMap[f.ID] = &c
	}
	t, err := s.newSheet(ctx, uid, ld.sheet.BaseID, clip(ld.sheet.Name+" (copy)", 100), ld.sheet.Icon, fields)
	if err != nil {
		s.fail(w, err, "copy table")
		return
	}
	if in.Rows {
		for _, rec := range ld.recs {
			data := map[string]any{}
			for k, v := range rec.Data {
				for old, nf := range idMap {
					if tables.Key(old) == k {
						data[tables.Key(nf.ID)] = v
					}
				}
			}
			nr := &store.Record{UserID: uid, SheetID: t.ID, Data: data, Position: rec.Position}
			if err := s.store.CreateRecord(ctx, nr); err != nil {
				s.fail(w, err, "copy rows")
				return
			}
		}
	}
	t, _ = s.store.GetSheet(ctx, uid, t.ID)
	writeJSON(w, http.StatusCreated, t)
}

// ---- fields ----

type fieldIn struct {
	Name        *string         `json:"name"`
	Type        *string         `json:"type"`
	Options     *tables.Options `json:"options"`
	Description *string         `json:"description"`
	Required    *bool           `json:"required"`
	Primary     *bool           `json:"primary"`
	After       int64           `json:"after"` // put it right after this field
}

// cleanField checks a field before it is saved: a free name, a known type
// and options that make sense for it.
func (s *Server) cleanField(ctx context.Context, uid int64, ld *loaded, f *tables.Field) error {
	f.Name = clip(strings.TrimSpace(f.Name), 100)
	if f.Name == "" {
		return &userErr{"give the field a name"}
	}
	if strings.ContainsAny(f.Name, "{}") {
		return &userErr{"a field name cannot have { or }"}
	}
	for _, o := range ld.fields {
		if o.ID != f.ID && strings.EqualFold(o.Name, f.Name) {
			return &userErr{"there is already a field " + o.Name}
		}
	}
	if !tables.ValidType(f.Type) {
		return &userErr{"unknown field type " + f.Type}
	}
	o := &f.Options
	keep := tables.Options{}
	switch f.Type {
	case tables.Number, tables.Percent:
		keep.Decimals = min(max(o.Decimals, 0), 8)
	case tables.Currency:
		keep.Decimals = min(max(o.Decimals, 0), 8)
		if keep.Decimals == 0 && o.Decimals == 0 {
			keep.Decimals = 2
		}
		keep.Currency = cleanCurrency(o.Currency)
		if keep.Currency == "" {
			keep.Currency = s.settingsOf(ctx, uid).Currency
		}
	case tables.Rating:
		keep.Max = o.Max
		if keep.Max < 1 || keep.Max > 10 {
			keep.Max = 5
		}
	case tables.Date:
		keep.Time, keep.Remind = o.Time, o.Remind
	case tables.Select, tables.Multi:
		keep.Choices = tables.CleanChoices(o.Choices)
	case tables.Link:
		keep = *o
		if _, err := s.store.GetSheet(ctx, uid, o.TableID); err != nil {
			return &userErr{"pick the table to link to"}
		}
	case tables.Lookup, tables.Rollup:
		link := ld.fieldByID(o.LinkField)
		if link == nil || link.Type != tables.Link {
			return &userErr{"pick the link field to look through"}
		}
		other, err := s.store.Fields(ctx, uid, link.Options.TableID)
		if err != nil {
			return err
		}
		found := false
		for _, x := range other {
			found = found || x.ID == o.Field
		}
		if !found {
			return &userErr{"pick the field of the linked rows"}
		}
		keep.LinkField, keep.Field = o.LinkField, o.Field
		if f.Type == tables.Rollup {
			keep.Fn = o.Fn
			if !oneOf(keep.Fn, tables.RollupFns) {
				keep.Fn = "sum"
			}
			keep.Decimals = min(max(o.Decimals, 0), 8)
		}
	case tables.Formula:
		eng := &tables.Sheet{Fields: ld.fields}
		_, refs, err := tables.Parse(o.Expr, eng.Resolver())
		if err != nil {
			return &userErr{"the formula: " + err.Error()}
		}
		for _, id := range refs {
			if id == f.ID && f.ID != 0 {
				return &userErr{"a formula cannot use itself"}
			}
		}
		keep.Expr = clip(strings.TrimSpace(o.Expr), 4000)
		keep.Decimals = min(max(o.Decimals, 0), 8)
	case tables.Pocket:
		if _, ok := kindBySlug(o.App); !ok {
			return &userErr{"pick what the field points at in your other apps"}
		}
		keep.App, keep.Single = o.App, o.Single
	}
	f.Options = keep
	if f.Primary && oneOf(f.Type, []string{tables.Link, tables.Attachment, tables.Checkbox, tables.Lookup, tables.Rollup, tables.Pocket}) {
		return &userErr{"the first field names the rows: it cannot be a " + typeWord(f.Type)}
	}
	return nil
}

func typeWord(t string) string {
	switch t {
	case tables.Link:
		return "link to another table"
	case tables.Attachment:
		return "file field"
	case tables.Multi:
		return "multiple choice"
	case tables.Pocket:
		return "link to another app"
	}
	return t
}

func (s *Server) handleCreateField(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ld, err := s.loader(uid).load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in fieldIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f := &tables.Field{TableID: ld.sheet.ID, Type: tables.Text}
	if in.Name != nil {
		f.Name = *in.Name
	}
	if in.Type != nil {
		f.Type = *in.Type
	}
	if in.Options != nil {
		f.Options = *in.Options
	}
	if in.Description != nil {
		f.Description = clip(strings.TrimSpace(*in.Description), 2000)
	}
	if in.Required != nil {
		f.Required = *in.Required
	}
	if strings.TrimSpace(f.Name) == "" {
		f.Name = freeName(ld, defaultFieldName(f.Type))
	}
	if err := s.cleanField(ctx, uid, ld, f); err != nil {
		s.userFail(w, err, "field")
		return
	}
	if in.After > 0 {
		if a := ld.fieldByID(in.After); a != nil {
			next := a.Position + 1
			for _, o := range ld.fields {
				if o.Position > a.Position && o.Position < next {
					next = o.Position
				}
			}
			f.Position = (a.Position + next) / 2
		}
	}
	if err := s.store.CreateField(ctx, uid, f); err != nil {
		s.fail(w, err, "create field")
		return
	}
	if f.Type == tables.Link {
		if err := s.addInverse(ctx, uid, ld, f); err != nil {
			s.fail(w, err, "link field")
			return
		}
	}
	s.store.TouchSheet(ctx, uid, ld.sheet.ID)
	writeJSON(w, http.StatusCreated, f)
}

// addInverse makes the field on the other side of a new link.
func (s *Server) addInverse(ctx context.Context, uid int64, ld *loaded, f *tables.Field) error {
	other, err := s.loader(uid).load(ctx, f.Options.TableID, false)
	if err != nil {
		return err
	}
	if other.sheet.ID == ld.sheet.ID {
		other = ld
		other.fields = append(other.fields, f)
	}
	inv := &tables.Field{TableID: other.sheet.ID, Name: freeName(other, ld.sheet.Name), Type: tables.Link,
		Options: tables.Options{TableID: ld.sheet.ID, Inverse: f.ID, Reverse: true}}
	if other.sheet.ID == ld.sheet.ID {
		inv.Name = freeName(other, f.Name+" (from)")
	}
	if err := s.store.CreateField(ctx, uid, inv); err != nil {
		return err
	}
	f.Options.Inverse = inv.ID
	return s.store.UpdateField(ctx, uid, f)
}

func defaultFieldName(t string) string {
	switch t {
	case tables.LongText:
		return "Notes"
	case tables.Number:
		return "Number"
	case tables.Currency:
		return "Price"
	case tables.Percent:
		return "Percent"
	case tables.Rating:
		return "Rating"
	case tables.Checkbox:
		return "Done"
	case tables.Date:
		return "Date"
	case tables.Select:
		return "Status"
	case tables.Multi:
		return "Tags"
	case tables.Email:
		return "Email"
	case tables.Phone:
		return "Phone"
	case tables.URL:
		return "Link"
	case tables.Attachment:
		return "Files"
	case tables.Link:
		return "Linked"
	case tables.Lookup:
		return "Lookup"
	case tables.Rollup:
		return "Total"
	case tables.Formula:
		return "Calculation"
	case tables.Pocket:
		return "In Pocket"
	case tables.Created:
		return "Added"
	case tables.Updated:
		return "Last changed"
	case tables.AutoNumber:
		return "No."
	}
	return "Field"
}

// freeName is name, or name 2, 3… when a field already has it.
func freeName(ld *loaded, name string) string {
	taken := func(n string) bool {
		for _, f := range ld.fields {
			if strings.EqualFold(f.Name, n) {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		n := name + " " + itoa(int64(i))
		if !taken(n) {
			return n
		}
	}
}

func (s *Server) handleUpdateField(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	f, err := s.store.GetField(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "field")
		return
	}
	ld, err := s.loader(uid).load(ctx, f.TableID, false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in fieldIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	old := *f
	if in.Name != nil {
		f.Name = *in.Name
	}
	if in.Type != nil && *in.Type != f.Type {
		if f.Type == tables.Link || *in.Type == tables.Link {
			writeErr(w, http.StatusBadRequest, "a link to another table cannot change into something else (or back): add a new field instead")
			return
		}
		f.Type = *in.Type
	}
	if in.Options != nil {
		opt := *in.Options
		if f.Type == tables.Link { // what links where is not changed here
			opt.TableID, opt.Inverse, opt.Reverse = f.Options.TableID, f.Options.Inverse, f.Options.Reverse
		}
		f.Options = opt
	} else if f.Type != old.Type {
		f.Options = tables.Options{}
	}
	if in.Description != nil {
		f.Description = clip(strings.TrimSpace(*in.Description), 2000)
	}
	if in.Required != nil {
		f.Required = *in.Required
	}
	if in.Primary != nil && *in.Primary {
		f.Primary = true
	}
	if err := s.cleanField(ctx, uid, ld, f); err != nil {
		s.userFail(w, err, "field")
		return
	}
	if f.Type != old.Type && !tables.Computed(f.Type) && !tables.Computed(old.Type) {
		if err := s.convertValues(ctx, uid, ld, &old, f); err != nil {
			s.fail(w, err, "change field type")
			return
		}
	}
	if f.Type == tables.Select || f.Type == tables.Multi {
		s.dropGoneChoices(ctx, uid, ld, f)
	}
	if err := s.store.UpdateField(ctx, uid, f); err != nil {
		s.fail(w, err, "save field")
		return
	}
	if f.Primary && !old.Primary {
		_ = s.store.SetPrimary(ctx, uid, f.TableID, f.ID)
	}
	if old.Name != f.Name {
		// formulas in this table that use the old name
		for _, o := range ld.fields {
			if o.Type == tables.Formula && o.ID != f.ID {
				if e := tables.RenameRefs(o.Options.Expr, old.Name, f.Name); e != o.Options.Expr {
					o.Options.Expr = e
					_ = s.store.UpdateField(ctx, uid, o)
				}
			}
		}
	}
	s.store.TouchSheet(ctx, uid, ld.sheet.ID)
	writeJSON(w, http.StatusOK, f)
}

// convertValues turns each row's value into the new type through its text
// ("12" → 12, "yes" → ticked, names → options).
func (s *Server) convertValues(ctx context.Context, uid int64, ld *loaded, old, f *tables.Field) error {
	k := tables.Key(f.ID)
	for _, r := range ld.recs {
		v, ok := r.Data[k]
		if !ok {
			continue
		}
		var in any = tables.Display(old, v)
		if old.Type == tables.Multi && f.Type == tables.Select {
			if names := strings.Split(tables.Display(old, v), ", "); len(names) > 0 {
				in = names[0]
			}
		}
		if old.Type == tables.Checkbox {
			in = tables.ToBool(v)
			if f.Type != tables.Checkbox && f.Type != tables.Number {
				if tables.ToBool(v) {
					in = "yes"
				} else {
					in = nil
				}
			}
		}
		nv, _ := tables.Normalize(f, in, true)
		if nv == nil {
			delete(r.Data, k)
		} else {
			r.Data[k] = nv
		}
		if err := s.store.SaveRecord(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// dropGoneChoices clears values whose option was removed.
func (s *Server) dropGoneChoices(ctx context.Context, uid int64, ld *loaded, f *tables.Field) {
	ok := map[string]bool{}
	for _, c := range f.Options.Choices {
		ok[c.ID] = true
	}
	k := tables.Key(f.ID)
	for _, r := range ld.recs {
		v, has := r.Data[k]
		if !has {
			continue
		}
		changed := false
		switch t := v.(type) {
		case string:
			if !ok[t] {
				delete(r.Data, k)
				changed = true
			}
		case []any:
			var keep []any
			for _, x := range t {
				if ok[tables.ToText(x)] {
					keep = append(keep, x)
				}
			}
			if len(keep) != len(t) {
				changed = true
				if len(keep) == 0 {
					delete(r.Data, k)
				} else {
					r.Data[k] = keep
				}
			}
		}
		if changed {
			_ = s.store.SaveRecord(ctx, r)
		}
	}
}

func (s *Server) handleDeleteField(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	f, err := s.store.GetField(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "field")
		return
	}
	if f.Primary {
		writeErr(w, http.StatusBadRequest, "the first field names the rows and cannot be deleted; make another field the first one first")
		return
	}
	if err := s.store.DeleteField(ctx, uid, f); err != nil {
		s.fail(w, err, "delete field")
		return
	}
	s.store.TouchSheet(ctx, uid, f.TableID)
	w.WriteHeader(http.StatusNoContent)
}

// ---- views ----

type viewIn struct {
	Name   *string           `json:"name"`
	Type   *string           `json:"type"`
	Config *store.ViewConfig `json:"config"`
	CopyOf int64             `json:"copy_of"`
}

func (s *Server) applyView(ctx context.Context, uid int64, v *store.View, in viewIn) error {
	if in.Name != nil {
		v.Name = clip(strings.TrimSpace(*in.Name), 100)
	}
	if v.Name == "" {
		return &userErr{"give the view a name"}
	}
	if in.Type != nil {
		if !oneOf(*in.Type, store.ViewTypes) {
			return &userErr{"a view is a grid, board, calendar or gallery"}
		}
		v.Type = *in.Type
	}
	if in.Config != nil {
		v.Config = *in.Config
		if len(v.Config.Filters) > 50 || len(v.Config.Sorts) > 10 {
			return &userErr{"too many filters or sorts"}
		}
	}
	fields, err := s.store.Fields(ctx, uid, v.SheetID)
	if err != nil {
		return err
	}
	first := func(types ...string) int64 {
		for _, f := range fields {
			if oneOf(f.Type, types) {
				return f.ID
			}
		}
		return 0
	}
	// a board needs a choice to stack by, a calendar a date
	if v.Type == "board" && v.Config.StackBy == 0 {
		v.Config.StackBy = first(tables.Select)
	}
	if v.Type == "calendar" && v.Config.DateField == 0 {
		v.Config.DateField = first(tables.Date, tables.Created)
	}
	if v.Type == "gallery" && v.Config.Cover == 0 {
		v.Config.Cover = first(tables.Attachment)
	}
	return nil
}

func (s *Server) handleCreateView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := s.store.GetSheet(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in viewIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	v := &store.View{UserID: uid, SheetID: t.ID, Type: "grid"}
	if in.CopyOf > 0 {
		src, err := s.store.GetView(ctx, uid, in.CopyOf)
		if err != nil || src.SheetID != t.ID {
			writeErr(w, http.StatusBadRequest, "there is no such view")
			return
		}
		v.Type, v.Config, v.Name = src.Type, src.Config, src.Name+" (copy)"
	}
	if in.Name == nil && v.Name == "" {
		typ := "grid"
		if in.Type != nil {
			typ = *in.Type
		}
		v.Name = map[string]string{"grid": "Grid", "board": "Board", "calendar": "Calendar", "gallery": "Gallery"}[typ]
	}
	if err := s.applyView(ctx, uid, v, in); err != nil {
		s.userFail(w, err, "view")
		return
	}
	if err := s.store.CreateView(ctx, v); err != nil {
		s.fail(w, err, "create view")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) handleUpdateView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetView(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "view")
		return
	}
	var in viewIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.applyView(ctx, uid, v, in); err != nil {
		s.userFail(w, err, "view")
		return
	}
	if err := s.store.UpdateView(ctx, v); err != nil {
		s.fail(w, err, "save view")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetView(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "view")
		return
	}
	if views, _ := s.store.Views(ctx, uid, v.SheetID); len(views) <= 1 {
		writeErr(w, http.StatusBadRequest, "a table keeps at least one view")
		return
	}
	if err := s.store.DeleteView(ctx, uid, v.ID); err != nil {
		s.fail(w, err, "delete view")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
