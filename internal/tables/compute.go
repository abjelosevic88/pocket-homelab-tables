package tables

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Record is one row as the engine sees it.
type Record struct {
	ID        int64
	Number    int64
	Data      map[string]any      // stored values by field id
	Links     map[int64][]LinkRef // link field id → the linked records
	CreatedAt string
	UpdatedAt string

	// Values are every field's value after Compute, by field id.
	Values map[string]any
}

// Sheet is a table's fields with its records, for working out values.
type Sheet struct {
	Fields  []*Field
	Records []*Record
	byID    map[int64]*Record
	fields  map[int64]*Field
	exprs   map[int64]Expr
	// Linked are the tables lookups and rollups read from, by table id.
	Linked map[int64]*Sheet
	Now    time.Time
}

// Key is how a field id is written in a record's values.
func Key(id int64) string { return strconv.FormatInt(id, 10) }

func (s *Sheet) index() {
	if s.fields != nil {
		return
	}
	s.fields = map[int64]*Field{}
	s.byID = map[int64]*Record{}
	s.exprs = map[int64]Expr{}
	for _, f := range s.Fields {
		s.fields[f.ID] = f
	}
	for _, r := range s.Records {
		s.byID[r.ID] = r
	}
	for _, f := range s.Fields {
		if f.Type == Formula {
			if e, _, err := Parse(f.Options.Expr, s.Resolver()); err == nil {
				s.exprs[f.ID] = e
			}
		}
	}
}

// Field finds a field by id.
func (s *Sheet) Field(id int64) *Field {
	s.index()
	return s.fields[id]
}

// Record finds a record by id.
func (s *Sheet) Record(id int64) *Record {
	s.index()
	return s.byID[id]
}

// Resolver maps a field name (not case-sensitive) to its id.
func (s *Sheet) Resolver() func(string) (int64, bool) {
	return func(name string) (int64, bool) {
		for _, f := range s.Fields {
			if strings.EqualFold(strings.TrimSpace(f.Name), strings.TrimSpace(name)) {
				return f.ID, true
			}
		}
		return 0, false
	}
}

// Primary is the field that names a record.
func (s *Sheet) Primary() *Field {
	for _, f := range s.Fields {
		if f.Primary {
			return f
		}
	}
	if len(s.Fields) > 0 {
		return s.Fields[0]
	}
	return nil
}

// Title is a record's name: its primary field as text.
func (s *Sheet) Title(r *Record) string {
	p := s.Primary()
	if p == nil || r == nil {
		return ""
	}
	v := r.Values[Key(p.ID)]
	if r.Values == nil {
		v = r.Data[Key(p.ID)]
	}
	return Display(p, v)
}

// Compute works out every record's Values: stored values, then the ones
// that come from the record itself (created, number), links, lookups and
// rollups, and last formulas (which may use all of those).
func (s *Sheet) Compute() {
	s.index()
	for _, r := range s.Records {
		s.computeOne(r)
	}
}

func (s *Sheet) computeOne(r *Record) {
	r.Values = make(map[string]any, len(s.Fields))
	for _, f := range s.Fields {
		k := Key(f.ID)
		switch f.Type {
		case Created:
			r.Values[k] = r.CreatedAt
		case Updated:
			r.Values[k] = r.UpdatedAt
		case AutoNumber:
			r.Values[k] = float64(r.Number)
		case Link:
			refs := r.Links[f.ID]
			if refs == nil {
				refs = []LinkRef{}
			}
			r.Values[k] = refs
		case Lookup, Rollup, Formula:
		default:
			if v, ok := r.Data[k]; ok && v != nil {
				r.Values[k] = v
			}
		}
	}
	for _, f := range s.Fields {
		if f.Type == Lookup || f.Type == Rollup {
			vals := s.lookup(r, f)
			if f.Type == Lookup {
				r.Values[Key(f.ID)] = vals
			} else {
				r.Values[Key(f.ID)] = Roll(f.Options.Fn, vals, len(r.Links[f.Options.LinkField]))
			}
		}
	}
	busy := map[int64]bool{}
	var get func(id int64) any
	get = func(id int64) any {
		f := s.fields[id]
		if f == nil {
			return nil
		}
		k := Key(id)
		if f.Type == Formula {
			if v, ok := r.Values[k]; ok {
				return v
			}
			if busy[id] {
				return nil // a formula that uses itself
			}
			busy[id] = true
			e := s.exprs[id]
			var v any
			if e != nil {
				v = Eval(e, &Env{Get: get, Now: s.Now})
			}
			if fv, ok := v.(float64); ok {
				v = math.Round(fv*1e9) / 1e9
			}
			r.Values[k] = v
			return v
		}
		return FormulaValue(f, r.Values[k])
	}
	for _, f := range s.Fields {
		if f.Type == Formula {
			get(f.ID)
		}
	}
	for k, v := range r.Values {
		if v == nil {
			delete(r.Values, k)
		}
	}
}

// lookup lists the values of a field of the records linked through a link field.
func (s *Sheet) lookup(r *Record, f *Field) []any {
	out := []any{}
	link := s.fields[f.Options.LinkField]
	if link == nil || link.Type != Link {
		return out
	}
	other := s.Linked[link.Options.TableID]
	if other == nil {
		return out
	}
	other.index()
	tf := other.fields[f.Options.Field]
	if tf == nil {
		return out
	}
	for _, ref := range r.Links[link.ID] {
		lr := other.byID[ref.ID]
		if lr == nil {
			continue
		}
		v := lr.Values[Key(tf.ID)]
		if lr.Values == nil {
			v = lr.Data[Key(tf.ID)]
		}
		out = append(out, LookupValues(tf, v)...)
	}
	return out
}

// LookupValues is a value as a lookup shows it: numbers stay numbers, yes/no
// stays yes/no, dates stay dates, the rest is text.
func LookupValues(f *Field, v any) []any {
	if v == nil {
		return nil
	}
	switch f.Type {
	case Number, Currency, Percent, Rating, AutoNumber:
		if n, ok := ToNumber(v); ok {
			return []any{n}
		}
	case Checkbox:
		return []any{ToBool(v)}
	case Date, Created, Updated:
		return []any{ToText(v)}
	case Multi:
		var out []any
		for _, id := range stringsOf(v) {
			if n := f.ChoiceName(id); n != "" {
				out = append(out, n)
			}
		}
		return out
	case Link:
		var out []any
		for _, l := range linkRefs(v) {
			out = append(out, l.Title)
		}
		return out
	case Pocket:
		var out []any
		for _, x := range toRefs(v, "") {
			out = append(out, x.Title)
		}
		return out
	case Attachment:
		var out []any
		for _, x := range toFiles(v) {
			out = append(out, x.Name)
		}
		return out
	case Lookup:
		if a, ok := v.([]any); ok {
			return a
		}
	case Formula, Rollup:
		if a, ok := v.([]any); ok {
			return a
		}
		return []any{v}
	}
	if s := Display(f, v); s != "" {
		return []any{s}
	}
	return nil
}

// FormulaValue is a field's value as a formula sees it.
func FormulaValue(f *Field, v any) any {
	if v == nil {
		switch {
		case Numeric(f.Type):
			return nil
		case f.Type == Checkbox:
			return false
		}
		return nil
	}
	switch f.Type {
	case Number, Currency, Percent, Rating, AutoNumber:
		if n, ok := ToNumber(v); ok {
			return n
		}
	case Checkbox:
		return ToBool(v)
	case Select:
		return f.ChoiceName(ToText(v))
	case Multi, Link, Pocket, Attachment:
		return LookupValues(f, v)
	case Lookup:
		return v
	}
	return v
}

// Roll does a rollup's sum (or count, average…) over looked-up values.
func Roll(fn string, vals []any, linked int) any {
	nums := []float64{}
	for _, v := range vals {
		if n, ok := v.(float64); ok {
			nums = append(nums, n)
		} else if s, ok := v.(string); ok && !reDate.MatchString(s) && !reDateTime.MatchString(s) {
			if n, ok := ParseNumber(s); ok && isPlainNumber(s) {
				nums = append(nums, n)
			}
		}
	}
	switch fn {
	case "sum", "":
		t := 0.0
		for _, n := range nums {
			t += n
		}
		return t
	case "avg":
		if len(nums) == 0 {
			return nil
		}
		t := 0.0
		for _, n := range nums {
			t += n
		}
		return t / float64(len(nums))
	case "min", "max":
		if len(nums) == 0 {
			return nil
		}
		m := nums[0]
		for _, n := range nums[1:] {
			if (fn == "min") == (n < m) {
				m = n
			}
		}
		return m
	case "count":
		return float64(linked)
	case "counta":
		c := 0
		for _, v := range vals {
			if !blank(v) {
				c++
			}
		}
		return float64(c)
	case "checked":
		c := 0
		for _, v := range vals {
			if b, ok := v.(bool); ok && b {
				c++
			}
		}
		return float64(c)
	case "earliest", "latest":
		var ds []string
		for _, v := range vals {
			if s, ok := v.(string); ok && (reDate.MatchString(s) || reDateTime.MatchString(s)) {
				ds = append(ds, s)
			}
		}
		if len(ds) == 0 {
			return nil
		}
		sort.Strings(ds)
		if fn == "earliest" {
			return ds[0]
		}
		return ds[len(ds)-1]
	case "list":
		seen := map[string]bool{}
		var parts []string
		for _, v := range vals {
			s := text(v)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			parts = append(parts, s)
		}
		return strings.Join(parts, ", ")
	}
	return nil
}

// Text is a record's value of a field as plain text (after Compute).
func (s *Sheet) Text(r *Record, f *Field) string {
	return Display(f, r.Values[Key(f.ID)])
}
