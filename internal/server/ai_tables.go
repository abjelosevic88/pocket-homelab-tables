package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- AI help ----
//
// Three things: describe what you want to keep track of and get the tables
// (fields, a few example rows, useful views); fill a column for many rows
// at once (sort into options, sum up a text, guess a missing value); and
// ask a question about a table.

const aiSystem = "You help a person keep their own small databases in Pocket Tables, an app like Airtable for non-technical people. Plain, friendly words; no jargon."

// readyAI is the person's AI or why there is none (and a slot to run in).
func (s *Server) readyAI(w http.ResponseWriter, r *http.Request) (*ai.Client, func(), bool) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off (Settings → AI)")
		return nil, nil, false
	}
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeErr(w, http.StatusServiceUnavailable, firstNonEmpty(why, "no AI is set up (Settings → AI)"))
		return nil, nil, false
	}
	select {
	case s.aiSem <- struct{}{}:
		return cl, func() { <-s.aiSem }, true
	case <-time.After(20 * time.Second):
		writeErr(w, http.StatusServiceUnavailable, "the AI is busy, try again in a moment")
		return nil, nil, false
	}
}

var designRules = `Answer as JSON: {"name": "...", "icon": "one emoji", "description": "one sentence", "tables": [{"name": "...", "icon": "one emoji", "fields": [...], "rows": [...], "views": [...]}]}.
Fields: {"name": "...", "type": "..."} with type one of: text, longtext, number, currency, percent, rating, checkbox, date, select, multiselect, email, phone, url, attachment, link, lookup, rollup, formula, pocket, created, autonumber.
- The FIRST field names each row (type text, or formula/autonumber) — never link, checkbox or attachment.
- select and multiselect have "choices": ["...", ...] (2 to 8 short names).
- date may have "options": {"time": true} for a time of day, {"remind": true} for a morning reminder.
- currency may have "options": {"currency": "EUR"}.
- link: {"type": "link", "link": "<other table name>"} (the other side appears there by itself, do not add it).
- rollup: {"type": "rollup", "through": "<link field>", "of": "<field there>", "fn": "sum|avg|min|max|count|counta|checked|list|earliest|latest"}; lookup the same without fn.
- formula: {"type": "formula", "expr": "..."} with fields in braces, e.g. {Price} * {Quantity}, IF({Done}, "✓", ""), DAYS({Due}, TODAY()).
- pocket points at a thing in the person's other apps: {"type": "pocket", "app": "contact|task|event|note|document|item|place|deal|book|wish|payment"}. Use it for people (contact) when the rows are about people the person knows.
Rows: 3 to 6 realistic example rows per table, values by field name (select by choice name, links by the other row's first field, dates YYYY-MM-DD from around ` + "TODAY" + `).
Views (optional, besides the grid every table gets): {"name": "...", "type": "board|calendar|gallery|grid", "by": "<field>"} — board by a select, calendar by a date, gallery by an attachment, grid grouped by a field.
Keep it small: 1 to 3 tables, 4 to 10 fields each. Names in the person's language.`

// handleAIDesign suggests a base for what the person describes (not saved yet).
func (s *Server) handleAIDesign(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Prompt string `json:"prompt"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "say what you want to keep track of")
		return
	}
	cl, done, ok := s.readyAI(w, r)
	if !ok {
		return
	}
	defer done()
	var spec specBase
	rules := strings.ReplaceAll(designRules, "TODAY", today())
	if err := cl.JSON(r.Context(), aiSystem+" "+rules, "What the person wants to keep track of:\n"+clip(in.Prompt, 4000), &spec); err != nil {
		writeErr(w, http.StatusBadGateway, "the AI did not answer well: "+err.Error())
		return
	}
	if len(spec.Tables) == 0 {
		writeErr(w, http.StatusBadGateway, "the AI did not suggest any table; say a little more")
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

// handleBuild makes a base (or tables in a base) from a description: a
// suggestion from the AI the person looked at.
func (s *Server) handleBuild(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Spec   specBase `json:"spec"`
		BaseID int64    `json:"base_id"`
		Rows   bool     `json:"rows"`
	}
	if err := decode(r, &in); err != nil || len(in.Spec.Tables) == 0 {
		writeErr(w, http.StatusBadRequest, "nothing to make")
		return
	}
	b, err := s.build(ctx, actorID(ctx), in.Spec, in.BaseID, in.Rows)
	if err != nil {
		s.userFail(w, err, "make")
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// handleAIFill fills one field for the rows of a view with what the AI
// works out from the rest of each row.
func (s *Server) handleAIFill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	f, err := s.store.GetField(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "field")
		return
	}
	if tables.Computed(f.Type) || f.Type == tables.Link || f.Type == tables.Attachment || f.Type == tables.Pocket {
		writeErr(w, http.StatusBadRequest, "the AI can fill text, numbers, dates, yes/no and options")
		return
	}
	var in struct {
		Instruction string  `json:"instruction"`
		OnlyEmpty   bool    `json:"only_empty"`
		View        int64   `json:"view"`
		IDs         []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Instruction) == "" {
		writeErr(w, http.StatusBadRequest, "say what the AI should put in "+f.Name)
		return
	}
	l := s.loader(uid)
	ld, err := l.load(ctx, f.TableID, false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var v *store.View
	if in.View > 0 {
		if vv, err := s.store.GetView(ctx, uid, in.View); err == nil && vv.SheetID == ld.sheet.ID {
			v = vv
		}
	}
	want := map[int64]bool{}
	for _, id := range in.IDs {
		want[id] = true
	}
	var todo []*tables.Record
	for _, x := range ld.view(v, "", l.now) {
		if len(want) > 0 && !want[x.ID] {
			continue
		}
		if in.OnlyEmpty && tables.Display(f, x.Values[tables.Key(f.ID)]) != "" {
			continue
		}
		todo = append(todo, x)
	}
	if len(todo) > 200 {
		todo = todo[:200]
	}
	if len(todo) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"filled": 0, "looked": 0})
		return
	}
	cl, done, ok := s.readyAI(w, r)
	if !ok {
		return
	}
	defer done()
	kind := map[string]string{tables.Number: "a number", tables.Currency: "a number (an amount)", tables.Percent: "a number (percent, 0-100)", tables.Rating: fmt.Sprintf("a whole number 1 to %d", max(f.Options.Max, 5)),
		tables.Checkbox: "true or false", tables.Date: "a date YYYY-MM-DD", tables.Email: "an email address", tables.Phone: "a phone number", tables.URL: "a web address", tables.LongText: "a few sentences"}[f.Type]
	if kind == "" {
		kind = "a short text"
	}
	if f.Type == tables.Select || f.Type == tables.Multi {
		var names []string
		for _, c := range f.Options.Choices {
			names = append(names, c.Name)
		}
		kind = "one of: " + strings.Join(names, " | ")
		if f.Type == tables.Multi {
			kind = "a list of any of: " + strings.Join(names, " | ")
		}
		if len(names) == 0 {
			kind = "a short label (the same label for rows that belong together)"
		}
	}
	filled := 0
	for start := 0; start < len(todo); start += 20 {
		chunk := todo[start:min(start+20, len(todo))]
		var rowsIn []map[string]any
		for _, x := range chunk {
			vals := map[string]string{}
			for _, o := range ld.fields {
				if o.ID == f.ID {
					continue
				}
				if t := ld.eng.Text(x, o); t != "" {
					vals[o.Name] = clip(t, 800)
				}
			}
			rowsIn = append(rowsIn, map[string]any{"id": x.ID, "values": vals})
		}
		b, _ := json.Marshal(rowsIn)
		var out struct {
			Values map[string]any `json:"values"`
		}
		prompt := fmt.Sprintf("Table %q. For each row, work out the field %q: %s\nThe value is %s. When it cannot be worked out, use null.\nRows:\n%s", ld.sheet.Name, f.Name, clip(in.Instruction, 2000), kind, b)
		if err := cl.JSON(ctx, aiSystem+` Answer as JSON {"values": {"<row id>": <value>, ...}} with every row id.`, prompt, &out); err != nil {
			if filled == 0 {
				writeErr(w, http.StatusBadGateway, "the AI did not answer well: "+err.Error())
				return
			}
			break
		}
		for _, x := range chunk {
			val, ok := out.Values[itoa(x.ID)]
			if !ok || val == nil {
				continue
			}
			rec := ld.byID[x.ID]
			changes, _, err := s.setValues(ctx, uid, ld, rec, map[string]any{tables.Key(f.ID): val}, true)
			if err != nil || len(changes) == 0 {
				continue
			}
			if s.store.SaveRecord(ctx, rec) == nil {
				filled++
				_ = s.store.AddHistory(ctx, uid, &store.History{RecordID: rec.ID, Kind: "change", Data: map[string]any{"changes": changes, "ai": true}})
			}
		}
	}
	if c := ld.sheet.Settings.Calendar; c != nil && c.On && filled > 0 {
		go s.syncSheetCalendar(context.WithoutCancel(ctx), uid, ld.sheet.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"filled": filled, "looked": len(todo)})
}

// handleAIAsk answers a question about a table (as a view shows it).
func (s *Server) handleAIAsk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Question string `json:"question"`
		View     int64  `json:"view"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Question) == "" {
		writeErr(w, http.StatusBadRequest, "ask something")
		return
	}
	l := s.loader(uid)
	ld, err := l.load(ctx, idParam(r, "id"), false)
	if err != nil {
		s.fail(w, err, "table")
		return
	}
	var v *store.View
	if in.View > 0 {
		if vv, err := s.store.GetView(ctx, uid, in.View); err == nil && vv.SheetID == ld.sheet.ID {
			v = vv
		}
	}
	cl, done, ok := s.readyAI(w, r)
	if !ok {
		return
	}
	defer done()
	data := string(s.tableCSV(ld, v, l.now))
	cut := ""
	if len(data) > 80000 {
		data, cut = data[:80000], "\n(the table goes on; only the first part is shown)"
	}
	prompt := fmt.Sprintf("Today is %s. The table %q as CSV:\n%s%s\n\nQuestion: %s", today(), ld.sheet.Name, data, cut, clip(in.Question, 2000))
	txt, err := cl.Complete(ctx, aiSystem+" Answer the question from the table only, in a few short lines; name the rows you mean. If the table cannot answer it, say so.", prompt, false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "the AI did not answer: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
}
