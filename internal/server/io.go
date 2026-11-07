package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- spreadsheets in and out ----

// parseSheet reads CSV or TSV (a file or a paste from Excel, Numbers,
// Google Sheets, Airtable): the separator is guessed from the first line.
func parseSheet(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		// an old Excel export: Windows-1250/1252 bytes as Latin-1 is better than nothing
		r := make([]rune, len(data))
		for i, b := range data {
			r[i] = rune(b)
		}
		data = []byte(string(r))
	}
	first, _, _ := bytes.Cut(data, []byte("\n"))
	sep := ','
	best := bytes.Count(first, []byte(","))
	for _, c := range []rune{'\t', ';'} {
		if n := bytes.Count(first, []byte(string(c))); n > best {
			sep, best = c, n
		}
	}
	rd := csv.NewReader(bytes.NewReader(data))
	rd.Comma = sep
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	var out [][]string
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, &userErr{"the file could not be read as a spreadsheet: " + err.Error()}
		}
		empty := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				empty = false
				break
			}
		}
		if !empty {
			out = append(out, rec)
		}
		if len(out) > 50001 {
			return nil, &userErr{"at most 50,000 rows at once"}
		}
	}
	if len(out) == 0 {
		return nil, &userErr{"the file is empty"}
	}
	return out, nil
}

// readUpload is the spreadsheet sent: a file ("file") or text ("text").
func (s *Server) readUpload(r *http.Request) ([]byte, string, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes)
		f, hdr, err := r.FormFile("file")
		if err != nil {
			if t := r.FormValue("text"); t != "" {
				return []byte(t), r.FormValue("name"), nil
			}
			return nil, "", &userErr{"send a file"}
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		name := strings.TrimSuffix(hdr.Filename, filepath.Ext(hdr.Filename))
		return b, firstNonEmpty(r.FormValue("name"), name), err
	}
	var in struct {
		Text string `json:"text"`
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		return nil, "", &userErr{"invalid json"}
	}
	return []byte(in.Text), in.Name, nil
}

// specFromSheet makes a table description from a spreadsheet: the first
// row names the columns, the types are guessed from the values.
func specFromSheet(name string, grid [][]string) specTable {
	head := grid[0]
	body := grid[1:]
	t := specTable{Name: firstNonEmpty(strings.TrimSpace(name), "Imported "+today())}
	seen := map[string]bool{}
	names := make([]string, len(head))
	for i, h := range head {
		h = strings.TrimSpace(strings.NewReplacer("{", "(", "}", ")").Replace(h))
		if h == "" {
			h = fmt.Sprintf("Column %d", i+1)
		}
		base := h
		for n := 2; seen[strings.ToLower(h)]; n++ {
			h = fmt.Sprintf("%s %d", base, n)
		}
		seen[strings.ToLower(h)] = true
		names[i] = h
		var vals []string
		for _, row := range body {
			if i < len(row) {
				vals = append(vals, row[i])
			}
		}
		typ, opt := tables.Infer(h, vals)
		if i == 0 && oneOf(typ, []string{tables.Checkbox, tables.Multi, tables.Attachment}) {
			typ, opt = tables.Text, tables.Options{}
		}
		sf := specField{Name: h, Type: typ, Options: opt}
		for _, c := range opt.Choices {
			sf.Choices = append(sf.Choices, c.Name)
		}
		sf.Options.Choices = nil
		t.Fields = append(t.Fields, sf)
	}
	for _, row := range body {
		m := map[string]any{}
		for i, c := range row {
			if i < len(names) && strings.TrimSpace(c) != "" {
				m[names[i]] = c
			}
		}
		t.Rows = append(t.Rows, m)
	}
	return t
}

// handleImportNew makes a new table in a base from a spreadsheet.
func (s *Server) handleImportNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.store.GetBase(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "base")
		return
	}
	data, name, err := s.readUpload(r)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	grid, err := parseSheet(data)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	t, n, err := s.importGrid(ctx, uid, b.ID, name, grid)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"table": t, "made": n})
}

func (s *Server) importGrid(ctx context.Context, uid, baseID int64, name string, grid [][]string) (*store.Sheet, int, error) {
	spec := specFromSheet(name, grid)
	before, _ := s.store.ListSheets(ctx, uid, baseID)
	built, err := s.build(ctx, uid, specBase{Tables: []specTable{spec}}, baseID, true)
	if err != nil {
		return nil, 0, err
	}
	known := map[int64]bool{}
	for _, t := range before {
		known[t.ID] = true
	}
	for _, t := range built.Tables {
		if !known[t.ID] {
			return t, t.Records, nil
		}
	}
	return nil, 0, &userErr{"nothing was made"}
}

// handleImportInto adds a spreadsheet's rows to a table: columns go to the
// fields with the same name; with add_fields the others become new fields.
func (s *Server) handleImportInto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ld, err := s.loader(uid).load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	data, _, err := s.readUpload(r)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	grid, err := parseSheet(data)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	addFields := queryBool(r, "add_fields") || r.FormValue("add_fields") == "1"
	head := grid[0]
	cols := make([]*tables.Field, len(head))
	matched := 0
	for i, h := range head {
		cols[i] = ld.field(strings.TrimSpace(h))
		if cols[i] != nil && !tables.Computed(cols[i].Type) {
			matched++
			continue
		}
		cols[i] = nil
		if addFields && strings.TrimSpace(h) != "" && !strings.ContainsAny(h, "{}") {
			var vals []string
			for _, row := range grid[1:] {
				if i < len(row) {
					vals = append(vals, row[i])
				}
			}
			typ, opt := tables.Infer(h, vals)
			f := &tables.Field{TableID: ld.sheet.ID, Name: strings.TrimSpace(h), Type: typ, Options: opt}
			if s.cleanField(ctx, uid, ld, f) == nil && s.store.CreateField(ctx, uid, f) == nil {
				ld.fields = append(ld.fields, f)
				cols[i] = f
				matched++
			}
		}
	}
	if matched == 0 {
		// no header row that matches: the columns go in field order
		var plain []*tables.Field
		for _, f := range ld.fields {
			if !tables.Computed(f.Type) && f.Type != tables.Attachment {
				plain = append(plain, f)
			}
		}
		for i := range cols {
			if i < len(plain) {
				cols[i] = plain[i]
			}
		}
	} else {
		grid = grid[1:]
	}
	made := 0
	for _, row := range grid {
		vals := map[string]any{}
		for i, c := range row {
			if i < len(cols) && cols[i] != nil && strings.TrimSpace(c) != "" {
				vals[tables.Key(cols[i].ID)] = c
			}
		}
		if len(vals) == 0 {
			continue
		}
		if _, err := s.addRecord(ctx, uid, ld, recordIn{Values: vals, Lenient: true}, ""); err != nil {
			s.userFail(w, err, "import")
			return
		}
		made++
	}
	if c := ld.sheet.Settings.Calendar; c != nil && c.On && made > 0 {
		go s.syncSheetCalendar(context.WithoutCancel(ctx), uid, ld.sheet.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"made": made})
}

// tableCSV is a table (as one view shows it) as CSV text.
func (s *Server) tableCSV(ld *loaded, v *store.View, now time.Time) []byte {
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf") // Excel reads UTF-8 with this
	cw := csv.NewWriter(&buf)
	fields := s.visibleFields(ld, v)
	head := make([]string, len(fields))
	for i, f := range fields {
		head[i] = f.Name
	}
	_ = cw.Write(head)
	for _, x := range ld.view(v, "", now) {
		row := make([]string, len(fields))
		for i, f := range fields {
			row[i] = ld.eng.Text(x, f)
		}
		_ = cw.Write(row)
	}
	cw.Flush()
	return buf.Bytes()
}

// visibleFields are a view's fields in its order, without the hidden ones.
func (s *Server) visibleFields(ld *loaded, v *store.View) []*tables.Field {
	fields := append([]*tables.Field(nil), ld.fields...)
	if v == nil {
		return fields
	}
	hidden := map[int64]bool{}
	for _, id := range v.Config.Hidden {
		hidden[id] = true
	}
	pos := map[int64]int{}
	for i, id := range v.Config.Order {
		pos[id] = i + 1
	}
	var out []*tables.Field
	for _, f := range fields {
		if !hidden[f.ID] || f.Primary {
			out = append(out, f)
		}
	}
	if len(pos) > 0 {
		// primary first, then the view's order, the rest after in table order
		idx := func(f *tables.Field) int {
			if f.Primary {
				return -1
			}
			if p, ok := pos[f.ID]; ok {
				return p
			}
			return 1 << 20
		}
		for i := 1; i < len(out); i++ {
			for j := i; j > 0 && idx(out[j]) < idx(out[j-1]); j-- {
				out[j], out[j-1] = out[j-1], out[j]
			}
		}
	}
	return out
}

func (s *Server) viewOf(ctx context.Context, uid int64, ld *loaded, r *http.Request) *store.View {
	if vid := int64(queryInt(r, "view")); vid > 0 {
		if v, err := s.store.GetView(ctx, uid, vid); err == nil && v.SheetID == ld.sheet.ID {
			return v
		}
	}
	return nil
}

func (s *Server) handleTableCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	l := s.loader(uid)
	ld, err := l.load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	v := s.viewOf(ctx, uid, ld, r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(ld.sheet.Name)+`.csv"`)
	_, _ = w.Write(s.tableCSV(ld, v, l.now))
}

func safeName(n string) string {
	n = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(n))
	if n == "" {
		return "table"
	}
	return clip(n, 80)
}

// ---- everything as JSON (and back) ----

type exportTable struct {
	Table   *store.Sheet    `json:"table"`
	Fields  []*tables.Field `json:"fields"`
	Views   []*store.View   `json:"views"`
	Records []exportRecord  `json:"records"`
}

type exportRecord struct {
	ID        int64          `json:"id"`
	Number    int64          `json:"number"`
	Data      map[string]any `json:"data"`
	Position  float64        `json:"position"`
	Source    string         `json:"source,omitempty"`
	SourceRef string         `json:"source_ref,omitempty"`
	CreatedAt string         `json:"created_at"`
	// Values are the values as text by field name, for reading elsewhere.
	Values map[string]string `json:"values"`
}

type exportBase struct {
	Base   *store.Base   `json:"base"`
	Tables []exportTable `json:"tables"`
}

type exportDoc struct {
	App      string          `json:"app"`
	Version  string          `json:"version"`
	Exported string          `json:"exported"`
	Bases    []exportBase    `json:"bases"`
	Links    []store.LinkRow `json:"links"`
}

func (s *Server) export(ctx context.Context, uid int64) (exportDoc, error) {
	doc := exportDoc{App: "pocket-tables", Version: Version, Exported: store.Now(), Bases: []exportBase{}, Links: []store.LinkRow{}}
	bases, err := s.tree(ctx, uid)
	if err != nil {
		return doc, err
	}
	var linkFields []int64
	l := s.loader(uid)
	for _, b := range bases {
		eb := exportBase{Base: b, Tables: []exportTable{}}
		for _, t := range b.Tables {
			ld, err := l.load(ctx, t.ID, false)
			if err != nil {
				return doc, err
			}
			views, _ := s.store.Views(ctx, uid, t.ID)
			et := exportTable{Table: ld.sheet, Fields: ld.fields, Views: views, Records: []exportRecord{}}
			for _, f := range ld.fields {
				if f.Type == tables.Link && !f.Options.Reverse {
					linkFields = append(linkFields, f.ID)
				}
			}
			for _, x := range ld.eng.Records {
				rec := ld.byID[x.ID]
				et.Records = append(et.Records, exportRecord{ID: x.ID, Number: x.Number, Data: rec.Data, Position: rec.Position, Source: rec.Source, SourceRef: rec.SourceRef, CreatedAt: rec.CreatedAt, Values: ld.named(x)})
			}
			eb.Tables = append(eb.Tables, et)
		}
		b.Tables = nil
		doc.Bases = append(doc.Bases, eb)
	}
	doc.Links, err = s.store.Links(ctx, uid, linkFields)
	return doc, err
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	doc, err := s.export(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-tables-%s.json"`, time.Now().Format("2006-01-02")))
	writeJSON(w, http.StatusOK, doc)
}

// importDoc brings back an export as new bases (ids are new; links,
// lookups, views and values are mapped to them).
func (s *Server) importDoc(ctx context.Context, uid int64, doc exportDoc) (map[string]int, error) {
	counts := map[string]int{"bases": 0, "tables": 0, "records": 0}
	sheetMap, fieldMap, recMap := map[int64]int64{}, map[int64]int64{}, map[int64]int64{}
	type pend struct {
		f   *tables.Field
		old tables.Options
	}
	var later []pend
	// bases, tables, fields
	for _, eb := range doc.Bases {
		if eb.Base == nil {
			continue
		}
		b := &store.Base{UserID: uid, Name: clip(firstNonEmpty(eb.Base.Name, "Imported"), 100), Icon: eb.Base.Icon, Color: eb.Base.Color, Description: eb.Base.Description}
		if err := s.store.CreateBase(ctx, b); err != nil {
			return counts, err
		}
		counts["bases"]++
		for _, et := range eb.Tables {
			if et.Table == nil {
				continue
			}
			t := &store.Sheet{UserID: uid, BaseID: b.ID, Name: clip(firstNonEmpty(et.Table.Name, "Table"), 100), Icon: et.Table.Icon, Description: et.Table.Description, Position: et.Table.Position}
			if err := s.store.CreateSheet(ctx, t); err != nil {
				return counts, err
			}
			sheetMap[et.Table.ID] = t.ID
			counts["tables"]++
			for _, f := range et.Fields {
				nf := *f
				nf.ID, nf.TableID = 0, t.ID
				if !tables.ValidType(nf.Type) {
					nf.Type = tables.Text
				}
				old := nf.Options
				if oneOf(nf.Type, []string{tables.Link, tables.Lookup, tables.Rollup}) {
					nf.Options = tables.Options{}
				}
				if err := s.store.CreateField(ctx, uid, &nf); err != nil {
					return counts, err
				}
				fieldMap[f.ID] = nf.ID
				if oneOf(nf.Type, []string{tables.Link, tables.Lookup, tables.Rollup}) {
					later = append(later, pend{&nf, old})
				}
			}
		}
	}
	// the options that name other fields and tables
	for _, p := range later {
		o := p.old
		o.TableID, o.Inverse, o.LinkField, o.Field = sheetMap[o.TableID], fieldMap[o.Inverse], fieldMap[o.LinkField], fieldMap[o.Field]
		p.f.Options = o
		_ = s.store.UpdateField(ctx, uid, p.f)
	}
	// rows and views
	for _, eb := range doc.Bases {
		for _, et := range eb.Tables {
			if et.Table == nil {
				continue
			}
			sid := sheetMap[et.Table.ID]
			for _, er := range et.Records {
				data := map[string]any{}
				for k, v := range er.Data {
					var id int64
					if _, err := fmt.Sscan(k, &id); err == nil && fieldMap[id] > 0 {
						data[tables.Key(fieldMap[id])] = v
					}
				}
				rec := &store.Record{UserID: uid, SheetID: sid, Data: data, Position: er.Position, Source: er.Source, CreatedAt: er.CreatedAt}
				if err := s.store.CreateRecord(ctx, rec); err != nil {
					return counts, err
				}
				recMap[er.ID] = rec.ID
				counts["records"]++
			}
			for _, v := range et.Views {
				nv := *v
				nv.ID, nv.UserID, nv.SheetID = 0, uid, sid
				c := &nv.Config
				mapIDs := func(ids []int64) []int64 {
					out := []int64{}
					for _, id := range ids {
						if fieldMap[id] > 0 {
							out = append(out, fieldMap[id])
						}
					}
					return out
				}
				c.Hidden, c.Order = mapIDs(c.Hidden), mapIDs(c.Order)
				c.GroupBy, c.StackBy, c.DateField, c.Cover, c.Color = fieldMap[c.GroupBy], fieldMap[c.StackBy], fieldMap[c.DateField], fieldMap[c.Cover], fieldMap[c.Color]
				var fl []tables.Filter
				for _, f := range c.Filters {
					if fieldMap[f.Field] > 0 {
						f.Field = fieldMap[f.Field]
						fl = append(fl, f)
					}
				}
				c.Filters = fl
				var so []tables.Sort
				for _, x := range c.Sorts {
					if fieldMap[x.Field] > 0 {
						x.Field = fieldMap[x.Field]
						so = append(so, x)
					}
				}
				c.Sorts = so
				widths, sums := map[string]int{}, map[string]string{}
				for k, w := range c.Widths {
					var id int64
					if _, err := fmt.Sscan(k, &id); err == nil && fieldMap[id] > 0 {
						widths[tables.Key(fieldMap[id])] = w
					}
				}
				for k, fn := range c.Summaries {
					var id int64
					if _, err := fmt.Sscan(k, &id); err == nil && fieldMap[id] > 0 {
						sums[tables.Key(fieldMap[id])] = fn
					}
				}
				c.Widths, c.Summaries = widths, sums
				if err := s.store.CreateView(ctx, &nv); err != nil {
					return counts, err
				}
			}
		}
	}
	for _, l := range doc.Links {
		if fieldMap[l.FieldID] > 0 && recMap[l.From] > 0 && recMap[l.To] > 0 {
			_, _ = s.store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO links (user_id, field_id, from_id, to_id) VALUES (?, ?, ?, ?)`, uid, fieldMap[l.FieldID], recMap[l.From], recMap[l.To])
		}
	}
	return counts, nil
}

// handleImport takes a Pocket Tables export (JSON) or a spreadsheet (CSV),
// which becomes a new base.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	data, name, err := s.readUpload(r)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	trim := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(trim) > 0 && trim[0] == '{' {
		var doc exportDoc
		if err := json.Unmarshal(trim, &doc); err != nil || doc.App != "pocket-tables" {
			writeErr(w, http.StatusBadRequest, "this is not a Pocket Tables export")
			return
		}
		counts, err := s.importDoc(ctx, uid, doc)
		if err != nil {
			s.fail(w, err, "import")
			return
		}
		writeJSON(w, http.StatusOK, counts)
		return
	}
	grid, err := parseSheet(data)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	b := &store.Base{UserID: uid, Name: clip(firstNonEmpty(name, "Imported"), 100), Icon: "📥", Color: BaseColors[0]}
	if err := s.store.CreateBase(ctx, b); err != nil {
		s.fail(w, err, "import")
		return
	}
	t, n, err := s.importGrid(ctx, uid, b.ID, name, grid)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bases": 1, "tables": 1, "records": n, "table": t})
}
