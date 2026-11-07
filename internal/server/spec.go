package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-tables/internal/store"
	"github.com/abjelosevic88/pocket-homelab-tables/internal/tables"
)

// ---- building a base from a description ----
//
// Templates, the AI's suggestion, a list from another app and the demo all
// describe a base the same way: tables with fields by name and rows by
// field name. Links name the other table; lookups and rollups name the
// link and the field; rows name linked rows by their title.

type specField struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Choices     []string       `json:"choices,omitempty"`
	Options     tables.Options `json:"options,omitempty"`
	Link        string         `json:"link,omitempty"`    // link: the other table's name
	Through     string         `json:"through,omitempty"` // lookup, rollup: the link field
	Of          string         `json:"of,omitempty"`      // lookup, rollup: the field there
	Fn          string         `json:"fn,omitempty"`      // rollup
	Expr        string         `json:"expr,omitempty"`    // formula
	App         string         `json:"app,omitempty"`     // pocket
	Description string         `json:"description,omitempty"`
}

type specView struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	By      string            `json:"by,omitempty"` // board: stack by; calendar: date; gallery: cover; grid: group by
	Sort    string            `json:"sort,omitempty"`
	Desc    bool              `json:"desc,omitempty"`
	Filters []specFilter      `json:"filters,omitempty"`
	Hide    []string          `json:"hide,omitempty"`
	Sums    map[string]string `json:"sums,omitempty"`
}

type specFilter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

type specTable struct {
	Name        string           `json:"name"`
	Icon        string           `json:"icon,omitempty"`
	Description string           `json:"description,omitempty"`
	Fields      []specField      `json:"fields"`
	Rows        []map[string]any `json:"rows,omitempty"`
	Views       []specView       `json:"views,omitempty"`
}

type specBase struct {
	Key         string      `json:"key,omitempty"`
	Name        string      `json:"name"`
	Icon        string      `json:"icon,omitempty"`
	Color       string      `json:"color,omitempty"`
	Description string      `json:"description,omitempty"`
	Tables      []specTable `json:"tables"`
}

// build makes a base (or, with into > 0, adds the tables to that base) from a spec.
func (s *Server) build(ctx context.Context, uid int64, spec specBase, into int64, rows bool) (*store.Base, error) {
	var b *store.Base
	var err error
	if into > 0 {
		if b, err = s.store.GetBase(ctx, uid, into); err != nil {
			return nil, err
		}
	} else {
		b = &store.Base{UserID: uid, Name: clip(firstNonEmpty(strings.TrimSpace(spec.Name), "New base"), 100), Icon: firstNonEmpty(spec.Icon, "📋"),
			Color: spec.Color, Description: clip(spec.Description, 2000), Template: spec.Key}
		if b.Color == "" || !reHex.MatchString(b.Color) {
			n, _ := s.store.ListBases(ctx, uid)
			b.Color = BaseColors[len(n)%len(BaseColors)]
		}
		if err := s.store.CreateBase(ctx, b); err != nil {
			return nil, err
		}
	}
	type made struct {
		spec   specTable
		sheet  *store.Sheet
		fields map[string]*tables.Field // by lower-case name
		order  []*tables.Field
	}
	var all []*made
	byName := map[string]*made{}
	// 1. tables with their plain fields
	for _, st := range spec.Tables {
		if len(all) >= 20 {
			break
		}
		name := clip(firstNonEmpty(strings.TrimSpace(st.Name), fmt.Sprintf("Table %d", len(all)+1)), 100)
		t := &store.Sheet{UserID: uid, BaseID: b.ID, Name: name, Icon: clip(st.Icon, 16), Description: clip(st.Description, 4000)}
		if err := s.store.CreateSheet(ctx, t); err != nil {
			return nil, err
		}
		m := &made{spec: st, sheet: t, fields: map[string]*tables.Field{}}
		all = append(all, m)
		byName[strings.ToLower(name)] = m
		for i, sf := range st.Fields {
			if i >= 100 || oneOf(sf.Type, []string{tables.Link, tables.Lookup, tables.Rollup, tables.Formula}) {
				continue
			}
			f := &tables.Field{TableID: t.ID, Name: sf.Name, Type: sf.Type, Options: sf.Options, Description: sf.Description, Primary: len(m.order) == 0}
			if !tables.ValidType(f.Type) {
				f.Type = tables.Text
			}
			if len(sf.Choices) > 0 {
				var cs []tables.Choice
				for _, c := range sf.Choices {
					cs = append(cs, tables.Choice{Name: c})
				}
				f.Options.Choices = cs
			}
			if sf.App != "" {
				f.Options.App = sf.App
			}
			ld := &loaded{sheet: t, fields: m.order}
			if f.Primary && oneOf(f.Type, []string{tables.Attachment, tables.Checkbox, tables.Pocket}) {
				f.Primary = false
			}
			if err := s.cleanField(ctx, uid, ld, f); err != nil {
				f.Type, f.Options = tables.Text, tables.Options{}
				if s.cleanField(ctx, uid, ld, f) != nil {
					continue
				}
			}
			if err := s.store.CreateField(ctx, uid, f); err != nil {
				return nil, err
			}
			m.fields[strings.ToLower(f.Name)] = f
			m.order = append(m.order, f)
		}
		if len(m.order) == 0 {
			f := &tables.Field{TableID: t.ID, Name: "Name", Type: tables.Text, Primary: true}
			if err := s.store.CreateField(ctx, uid, f); err != nil {
				return nil, err
			}
			m.fields["name"] = f
			m.order = append(m.order, f)
		}
	}
	// 2. links (with their other side), then lookups and rollups, then formulas
	for _, kind := range []string{tables.Link, tables.Lookup, tables.Formula} {
		for _, m := range all {
			for _, sf := range m.spec.Fields {
				t := sf.Type
				if t == tables.Rollup {
					t = tables.Lookup
				}
				if t != kind || m.fields[strings.ToLower(sf.Name)] != nil {
					continue
				}
				f := &tables.Field{TableID: m.sheet.ID, Name: sf.Name, Type: sf.Type, Description: sf.Description}
				switch sf.Type {
				case tables.Link:
					other := byName[strings.ToLower(sf.Link)]
					if other == nil {
						continue
					}
					f.Options = tables.Options{TableID: other.sheet.ID, Single: sf.Options.Single}
				case tables.Lookup, tables.Rollup:
					link := m.fields[strings.ToLower(sf.Through)]
					if link == nil || link.Type != tables.Link {
						continue
					}
					var other *made
					for _, o := range all {
						if o.sheet.ID == link.Options.TableID {
							other = o
						}
					}
					if other == nil || other.fields[strings.ToLower(sf.Of)] == nil {
						continue
					}
					f.Options = tables.Options{LinkField: link.ID, Field: other.fields[strings.ToLower(sf.Of)].ID, Fn: sf.Fn, Decimals: sf.Options.Decimals}
				case tables.Formula:
					f.Options = tables.Options{Expr: sf.Expr, Decimals: sf.Options.Decimals}
				}
				ld := &loaded{sheet: m.sheet, fields: m.order}
				if err := s.cleanField(ctx, uid, ld, f); err != nil {
					continue
				}
				if err := s.store.CreateField(ctx, uid, f); err != nil {
					return nil, err
				}
				m.fields[strings.ToLower(f.Name)] = f
				m.order = append(m.order, f)
				if f.Type == tables.Link {
					other := byName[strings.ToLower(sf.Link)]
					if err := s.addInverse(ctx, uid, ld, f); err != nil {
						return nil, err
					}
					inv, _ := s.store.GetField(ctx, uid, f.Options.Inverse)
					if inv != nil && other != nil {
						other.fields[strings.ToLower(inv.Name)] = inv
						other.order = append(other.order, inv)
					}
				}
			}
		}
	}
	// 3. rows: plain values first, links once every table has its rows
	type later struct {
		m    *made
		id   int64
		vals map[string]any
	}
	var links []later
	if rows {
		for _, m := range all {
			ld := &loaded{sheet: m.sheet, fields: m.order, byID: map[int64]*store.Record{}, eng: &tables.Sheet{Fields: m.order}}
			for i, row := range m.spec.Rows {
				if i >= 2000 {
					break
				}
				plain := map[string]any{}
				lv := map[string]any{}
				for k, v := range row {
					f := m.fields[strings.ToLower(k)]
					if f == nil {
						continue
					}
					if f.Type == tables.Link {
						lv[k] = v
					} else {
						plain[k] = v
					}
				}
				rec := &store.Record{UserID: uid, SheetID: m.sheet.ID, Data: map[string]any{}, Source: "tables"}
				if _, _, err := s.setValues(ctx, uid, ld, rec, plain, true); err != nil {
					return nil, err
				}
				if c, ok := row["_created"].(string); ok {
					rec.CreatedAt = c
				}
				if err := s.store.CreateRecord(ctx, rec); err != nil {
					return nil, err
				}
				if len(lv) > 0 {
					links = append(links, later{m, rec.ID, lv})
				}
			}
		}
		for _, l := range links {
			ld, err := s.loader(uid).load(ctx, l.m.sheet.ID, false)
			if err != nil {
				return nil, err
			}
			rec := ld.byID[l.id]
			if rec == nil {
				continue
			}
			if _, _, err := s.setValues(ctx, uid, ld, rec, l.vals, true); err != nil {
				return nil, err
			}
		}
	}
	// 4. views: a grid first, then the ones asked for
	for _, m := range all {
		// every table starts with a plain grid of all its rows
		views := m.spec.Views
		if len(views) == 0 || views[0].Type != "grid" || len(views[0].Filters) > 0 || views[0].By != "" {
			views = append([]specView{{Name: "Grid", Type: "grid"}}, views...)
		}
		for _, sv := range views {
			v := &store.View{UserID: uid, SheetID: m.sheet.ID, Name: clip(firstNonEmpty(sv.Name, "View"), 100), Type: sv.Type}
			if !oneOf(v.Type, store.ViewTypes) {
				v.Type = "grid"
			}
			id := func(name string) int64 {
				if f := m.fields[strings.ToLower(name)]; f != nil {
					return f.ID
				}
				return 0
			}
			switch v.Type {
			case "board":
				v.Config.StackBy = id(sv.By)
			case "calendar":
				v.Config.DateField = id(sv.By)
			case "gallery":
				v.Config.Cover = id(sv.By)
			default:
				v.Config.GroupBy = id(sv.By)
			}
			if sv.Sort != "" && id(sv.Sort) > 0 {
				v.Config.Sorts = []tables.Sort{{Field: id(sv.Sort), Desc: sv.Desc}}
			}
			for _, fl := range sv.Filters {
				if fid := id(fl.Field); fid > 0 {
					val := fl.Value
					if f := m.fields[strings.ToLower(fl.Field)]; f != nil && (f.Type == tables.Select || f.Type == tables.Multi) {
						if name, ok := val.(string); ok {
							for _, c := range f.Options.Choices {
								if strings.EqualFold(c.Name, name) {
									val = c.ID
								}
							}
						}
					}
					v.Config.Filters = append(v.Config.Filters, tables.Filter{Field: fid, Op: fl.Op, Value: val})
				}
			}
			for _, h := range sv.Hide {
				if fid := id(h); fid > 0 {
					v.Config.Hidden = append(v.Config.Hidden, fid)
				}
			}
			if len(sv.Sums) > 0 {
				v.Config.Summaries = map[string]string{}
				for k, fn := range sv.Sums {
					if fid := id(k); fid > 0 {
						v.Config.Summaries[tables.Key(fid)] = fn
					}
				}
			}
			if err := s.applyView(ctx, uid, v, viewIn{}); err != nil {
				continue
			}
			if err := s.store.CreateView(ctx, v); err != nil {
				return nil, err
			}
		}
	}
	b.Tables, _ = s.store.ListSheets(ctx, uid, b.ID)
	return b, nil
}
