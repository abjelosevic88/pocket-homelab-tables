package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- rows ----

func (s *Server) handleListRecords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	deleted := queryBool(r, "deleted")
	l := s.loader(uid)
	ld, err := l.load(ctx, idParam(r, "id"), deleted)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var v *store.View
	if vid := int64(queryInt(r, "view")); vid > 0 && !deleted {
		v, err = s.store.GetView(ctx, uid, vid)
		if err != nil || v.SheetID != ld.sheet.ID {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
	}
	recs := ld.view(v, r.URL.Query().Get("q"), l.now)
	total := len(recs)
	limit, offset := queryInt(r, "limit"), queryInt(r, "offset")
	if offset > 0 && offset < len(recs) {
		recs = recs[offset:]
	} else if offset >= len(recs) && offset > 0 {
		recs = nil
	}
	if limit > 0 && limit < len(recs) {
		recs = recs[:limit]
	}
	comments := s.store.CommentCounts(ctx, uid, ld.sheet.ID)
	named := queryBool(r, "named") || linkApp(ctx) != ""
	out := make([]recordOut, 0, len(recs))
	for _, x := range recs {
		o := ld.out(x, named)
		o.Comments = comments[x.ID]
		out = append(out, o)
	}
	resp := map[string]any{"records": out, "total": total, "table": ld.sheet.Name}
	if v != nil && len(v.Config.Summaries) > 0 {
		all := ld.view(v, r.URL.Query().Get("q"), l.now)
		sums := map[string]any{}
		for k, fn := range v.Config.Summaries {
			if f := ld.field(k); f != nil && fn != "" {
				sums[k] = ld.eng.Summary(all, f, fn)
			}
		}
		resp["summaries"] = sums
	}
	writeJSON(w, http.StatusOK, resp)
}

type recordIn struct {
	Values  map[string]any `json:"values"`
	Fields  map[string]any `json:"fields"` // the same, as Airtable names it
	Lenient bool           `json:"typecast"`
	After   int64          `json:"after"`  // put it right after this row
	Before  int64          `json:"before"` // or right before
	Ref     string         `json:"ref"`    // another app's reference: never added twice
}

func (in *recordIn) values() map[string]any {
	if in.Values == nil {
		in.Values = map[string]any{}
	}
	for k, v := range in.Fields {
		in.Values[k] = v
	}
	return in.Values
}

// position between two neighbours.
func between(ld *loaded, after, before int64) float64 {
	pos := func(id int64) (float64, bool) {
		if r := ld.byID[id]; r != nil {
			return r.Position, true
		}
		return 0, false
	}
	if p, ok := pos(after); ok {
		next := p + 2
		for _, r := range ld.recs {
			if r.Position > p && r.Position < next {
				next = r.Position
			}
		}
		return (p + next) / 2
	}
	if p, ok := pos(before); ok {
		prev := p - 2
		for _, r := range ld.recs {
			if r.Position < p && r.Position > prev {
				prev = r.Position
			}
		}
		return (p + prev) / 2
	}
	return 0
}

// addRecord makes a row from values; app is who added it ("" = the person).
func (s *Server) addRecord(ctx context.Context, uid int64, ld *loaded, in recordIn, app string) (*store.Record, error) {
	rec := &store.Record{UserID: uid, SheetID: ld.sheet.ID, Data: map[string]any{}, Source: app, SourceRef: clip(strings.TrimSpace(in.Ref), 200)}
	if rec.SourceRef != "" {
		if old, err := s.store.RecordBySource(ctx, uid, ld.sheet.ID, rec.SourceRef); err == nil {
			return old, store.ErrConflict
		}
	}
	rec.Position = between(ld, in.After, in.Before)
	changes, pending, err := s.setValues(ctx, uid, ld, rec, in.values(), in.Lenient || app != "")
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateRecord(ctx, rec); err != nil {
		return nil, err
	}
	if err := s.flushLinks(ctx, uid, rec, pending); err != nil {
		return nil, err
	}
	_ = changes
	_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: rec.ID, Kind: "created", App: app})
	return rec, nil
}

func (s *Server) handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ld, err := s.loader(uid).load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in recordIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	app := linkApp(ctx)
	rec, err := s.addRecord(ctx, uid, ld, in, app)
	if err == store.ErrConflict && rec != nil {
		// another app sent the same thing twice: the row it made already
		s.writeRecord(w, ctx, uid, ld.sheet.ID, rec.ID, http.StatusOK)
		return
	}
	if err != nil {
		s.userFail(w, err, "add row")
		return
	}
	s.afterWrite(ctx, uid, ld.sheet.ID, rec.ID, "tables.record_created", nil)
	s.writeRecord(w, ctx, uid, ld.sheet.ID, rec.ID, http.StatusCreated)
}

// writeRecord answers with one row, worked out fresh.
func (s *Server) writeRecord(w http.ResponseWriter, ctx context.Context, uid, sheetID, id int64, code int) {
	ld, err := s.loader(uid).load(ctx, sheetID, false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	x := ld.eng.Record(id)
	if x == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, code, ld.out(x, linkApp(ctx) != ""))
}

// afterWrite tells the box and keeps the calendar up to date.
func (s *Server) afterWrite(ctx context.Context, uid, sheetID, id int64, topic string, changes []change) {
	ctx = context.WithoutCancel(ctx)
	go func() {
		ld, err := s.loader(uid).load(ctx, sheetID, false)
		if err != nil {
			return
		}
		if topic != "" {
			s.recordEvent(ctx, uid, topic, ld, id, changes)
		}
		if c := ld.sheet.Settings.Calendar; c != nil && c.On {
			s.syncRecordCalendar(ctx, uid, ld, id)
		}
	}()
}

// handleBatch adds many rows at once (a paste from a spreadsheet).
func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ld, err := s.loader(uid).load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in struct {
		Records []recordIn `json:"records"`
		Lenient bool       `json:"typecast"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(in.Records) > 5000 {
		writeErr(w, http.StatusBadRequest, "at most 5000 rows at once")
		return
	}
	made := 0
	var ids []int64
	for _, ri := range in.Records {
		ri.Lenient = ri.Lenient || in.Lenient
		rec, err := s.addRecord(ctx, uid, ld, ri, linkApp(ctx))
		if err == store.ErrConflict {
			continue
		}
		if err != nil {
			s.userFail(w, err, "add rows")
			return
		}
		made++
		ids = append(ids, rec.ID)
	}
	if c := ld.sheet.Settings.Calendar; c != nil && c.On {
		go s.syncSheetCalendar(context.WithoutCancel(ctx), uid, ld.sheet.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"made": made, "ids": ids})
}

// recordPage is a row with all a row page needs.
type recordPage struct {
	Record  recordOut        `json:"record"`
	Table   *store.Sheet     `json:"table"`
	Fields  []*tables.Field  `json:"fields"`
	History []*store.History `json:"history"`
	Made    []*store.Made    `json:"made"`
	Prev    int64            `json:"prev,omitempty"`
	Next    int64            `json:"next,omitempty"`
}

func (s *Server) ownRecord(w http.ResponseWriter, r *http.Request) (*store.Record, bool) {
	rec, err := s.store.GetRecord(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "row")
		return nil, false
	}
	return rec, true
}

func (s *Server) handleGetRecord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
		return
	}
	ld, err := s.loader(uid).load(ctx, rec.SheetID, rec.DeletedAt != "")
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	x := ld.eng.Record(rec.ID)
	if x == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	p := recordPage{Record: ld.out(x, true), Table: ld.sheet, Fields: ld.fields}
	p.History, _ = s.store.HistoryOf(ctx, uid, rec.ID)
	p.Made, _ = s.store.MadeFor(ctx, uid, rec.ID, "")
	if linkApp(ctx) == "" {
		for i, y := range ld.eng.Records {
			if y.ID == rec.ID {
				if i > 0 {
					p.Prev = ld.eng.Records[i-1].ID
				}
				if i+1 < len(ld.eng.Records) {
					p.Next = ld.eng.Records[i+1].ID
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdateRecord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
		return
	}
	if app := linkApp(ctx); app != "" && rec.Source != app {
		writeErr(w, http.StatusForbidden, "another app may change only the rows it added")
		return
	}
	if rec.DeletedAt != "" {
		writeErr(w, http.StatusBadRequest, "the row is in the bin: bring it back first")
		return
	}
	ld, err := s.loader(uid).load(ctx, rec.SheetID, false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in recordIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	changes, _, err := s.setValues(ctx, uid, ld, rec, in.values(), in.Lenient || linkApp(ctx) != "")
	if err != nil {
		s.userFail(w, err, "save row")
		return
	}
	if p := between(ld, in.After, in.Before); p != 0 {
		rec.Position = p
	}
	if err := s.store.SaveRecord(ctx, rec); err != nil {
		s.fail(w, err, "save row")
		return
	}
	if len(changes) > 0 {
		data := map[string]any{"changes": changes}
		_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: rec.ID, Kind: "change", Data: data, App: linkApp(ctx)})
		s.afterWrite(ctx, uid, rec.SheetID, rec.ID, "tables.record_updated", changes)
	}
	s.writeRecord(w, ctx, uid, rec.SheetID, rec.ID, http.StatusOK)
}

func (s *Server) handleDeleteRecord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
		return
	}
	s.deleteRows(ctx, uid, rec.SheetID, []int64{rec.ID})
	w.WriteHeader(http.StatusNoContent)
}

// deleteRows moves rows to the bin, takes them off the calendar and tells the box.
func (s *Server) deleteRows(ctx context.Context, uid, sheetID int64, ids []int64) int {
	ld, err := s.loader(uid).load(ctx, sheetID, false)
	if err == nil {
		for _, id := range ids {
			if ld.byID[id] != nil {
				s.recordEvent(ctx, uid, "tables.record_deleted", ld, id, nil)
			}
		}
	}
	n, _ := s.store.DeleteRecords(ctx, uid, ids)
	go func() {
		bg := context.WithoutCancel(ctx)
		for _, id := range ids {
			s.unsyncRecord(bg, uid, id)
		}
	}()
	return n
}

func (s *Server) handleDeleteRecords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	t, err := s.store.GetSheet(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids := s.store.OwnRecords(ctx, uid, t.ID, in.IDs)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": s.deleteRows(ctx, uid, t.ID, ids)})
}

func (s *Server) handleRestoreRecord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
		return
	}
	if err := s.store.RestoreRecord(ctx, uid, rec.ID); err != nil {
		s.fail(w, err, "bring back")
		return
	}
	_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: rec.ID, Kind: "restored"})
	s.afterWrite(ctx, uid, rec.SheetID, rec.ID, "tables.record_created", nil)
	s.writeRecord(w, ctx, uid, rec.SheetID, rec.ID, http.StatusOK)
}

func (s *Server) handlePurgeRecord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
		return
	}
	if rec.DeletedAt == "" {
		writeErr(w, http.StatusBadRequest, "only rows in the bin can be removed for good")
		return
	}
	if err := s.store.PurgeRecord(ctx, uid, rec.ID); err != nil {
		s.fail(w, err, "remove")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDuplicateRecord(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
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
	vals := map[string]any{}
	for k, v := range rec.Data {
		vals[k] = v
	}
	for _, f := range ld.fields {
		if f.Type == tables.Link {
			var ids []any
			for _, l := range x.Links[f.ID] {
				ids = append(ids, float64(l.ID))
			}
			vals[tables.Key(f.ID)] = ids
		}
	}
	nr, err := s.addRecord(ctx, uid, ld, recordIn{Values: vals, After: rec.ID, Lenient: true}, "")
	if err != nil {
		s.userFail(w, err, "copy row")
		return
	}
	s.afterWrite(ctx, uid, rec.SheetID, nr.ID, "tables.record_created", nil)
	s.writeRecord(w, ctx, uid, rec.SheetID, nr.ID, http.StatusCreated)
}

// ---- comments ----

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rec, ok := s.ownRecord(w, r)
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t := clip(strings.TrimSpace(in.Text), 10000)
	if t == "" {
		writeErr(w, http.StatusBadRequest, "write something")
		return
	}
	h := &store.History{RecordID: rec.ID, Kind: "comment", Text: t, App: linkApp(ctx)}
	if err := s.store.AddHistory(ctx, uid, h); err != nil {
		s.fail(w, err, "comment")
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteComment(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "comment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- files ----

func (s *Server) filePath(id string) string { return filepath.Join(s.cfg.DataDir, "files", id) }

func newFileID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// handleUpload keeps a file; the page then puts it in a row's file field.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
	src, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "send the file as \"file\" (at most "+itoa(s.cfg.MaxBodyBytes>>20)+" MB)")
		return
	}
	defer src.Close()
	meta, err := s.keepFile(ctx, uid, hdr.Filename, hdr.Header.Get("Content-Type"), src)
	if err != nil {
		s.fail(w, err, "upload")
		return
	}
	writeJSON(w, http.StatusCreated, meta)
}

func (s *Server) keepFile(ctx context.Context, uid int64, name, typ string, src io.Reader) (*store.FileMeta, error) {
	if err := os.MkdirAll(filepath.Join(s.cfg.DataDir, "files"), 0o755); err != nil {
		return nil, err
	}
	id := newFileID()
	dst, err := os.Create(s.filePath(id))
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(dst, src)
	dst.Close()
	if err != nil {
		os.Remove(s.filePath(id))
		return nil, err
	}
	name = clip(filepath.Base(strings.ReplaceAll(name, "\\", "/")), 200)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	if typ == "" || typ == "application/octet-stream" {
		if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
			typ = t
		}
	}
	meta := &store.FileMeta{ID: id, UserID: uid, Name: name, Mime: clip(typ, 120), Size: n}
	if err := s.store.AddFile(ctx, meta); err != nil {
		os.Remove(s.filePath(id))
		return nil, err
	}
	return meta, nil
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	meta, err := s.store.GetFile(ctx, actorID(ctx), chiParam(r, "id"))
	if err != nil {
		s.fail(w, err, "file")
		return
	}
	f, err := os.Open(s.filePath(meta.ID))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	defer f.Close()
	typ := meta.Mime
	inline := strings.HasPrefix(typ, "image/") || typ == "application/pdf" || strings.HasPrefix(typ, "video/") || strings.HasPrefix(typ, "audio/")
	if typ == "" || strings.Contains(typ, "html") || strings.Contains(typ, "svg") || strings.Contains(typ, "javascript") {
		typ, inline = "application/octet-stream", false // never run what someone uploaded
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	disp := "attachment"
	if inline && !queryBool(r, "dl") {
		disp = "inline"
	}
	w.Header().Set("Content-Disposition", disp+`; filename="`+strings.ReplaceAll(meta.Name, `"`, "")+`"`)
	fi, _ := f.Stat()
	http.ServeContent(w, r, "", fi.ModTime(), f)
}
