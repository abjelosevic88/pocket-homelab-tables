package tables

import (
	"sort"
	"strings"
	"time"
)

// Filter is one condition of a view: a field, how to compare and with what.
type Filter struct {
	Field int64  `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// Sort orders a view by a field.
type Sort struct {
	Field int64 `json:"field"`
	Desc  bool  `json:"desc"`
}

// Ops a filter may use. Which fit a field is up to the page; the engine
// answers every op for every field as sensibly as it can.
var Ops = []string{"contains", "not_contains", "is", "is_not", "empty", "not_empty", "gt", "gte", "lt", "lte",
	"before", "after", "on_or_before", "on_or_after", "within", "checked", "unchecked", "any_of", "none_of", "has_all"}

// kind is how a field's values compare: text, number, date, bool, choice, list.
func kind(f *Field, v any) string {
	switch f.Type {
	case Number, Currency, Percent, Rating, AutoNumber:
		return "number"
	case Date, Created, Updated:
		return "date"
	case Checkbox:
		return "bool"
	case Select:
		return "choice"
	case Multi:
		return "multi"
	case Link, Lookup, Pocket, Attachment:
		return "list"
	case Formula, Rollup:
		switch t := v.(type) {
		case float64:
			return "number"
		case bool:
			return "bool"
		case string:
			if reDate.MatchString(t) || reDateTime.MatchString(t) {
				return "date"
			}
		}
	}
	return "text"
}

// Match reports whether a record passes a view's filters (all of them, or
// with any set any one of them).
func (s *Sheet) Match(r *Record, filters []Filter, any bool, now time.Time) bool {
	s.index()
	n := 0
	for _, fl := range filters {
		f := s.fields[fl.Field]
		if f == nil || fl.Op == "" {
			continue
		}
		if needsValue(fl.Op) && blank(fl.Value) {
			continue // a condition still being written
		}
		n++
		ok := matchOne(f, r.Values[Key(f.ID)], fl, now)
		if any && ok {
			return true
		}
		if !any && !ok {
			return false
		}
	}
	return !any || n == 0
}

func needsValue(op string) bool {
	switch op {
	case "empty", "not_empty", "checked", "unchecked":
		return false
	}
	return true
}

func matchOne(f *Field, v any, fl Filter, now time.Time) bool {
	disp := strings.ToLower(Display(f, v))
	empty := disp == "" && (f.Type != Checkbox || !ToBool(v))
	switch fl.Op {
	case "empty":
		return empty
	case "not_empty":
		return !empty
	case "checked":
		return ToBool(v)
	case "unchecked":
		return !ToBool(v)
	}
	k := kind(f, v)
	want := fl.Value
	switch k {
	case "number":
		n, ok := ToNumber(v)
		w, wok := ToNumber(want)
		if !wok {
			return true
		}
		switch fl.Op {
		case "is":
			return ok && n == w
		case "is_not":
			return !ok || n != w
		case "gt":
			return ok && n > w
		case "gte":
			return ok && n >= w
		case "lt":
			return ok && n < w
		case "lte":
			return ok && n <= w
		}
	case "date":
		d := dateOnly(ToText(v))
		if fl.Op == "within" {
			from, to := within(ToText(want), now)
			return d != "" && d >= from && d <= to
		}
		w := RelDate(ToText(want), now)
		if w == "" {
			return true
		}
		switch fl.Op {
		case "is":
			return d == w
		case "is_not":
			return d != w
		case "before", "lt":
			return d != "" && d < w
		case "after", "gt":
			return d != "" && d > w
		case "on_or_before", "lte":
			return d != "" && d <= w
		case "on_or_after", "gte":
			return d != "" && d >= w
		}
	case "bool":
		b := ToBool(v)
		switch fl.Op {
		case "is":
			return b == ToBool(want)
		case "is_not":
			return b != ToBool(want)
		}
	case "choice", "multi":
		have := stringsOf(v)
		if k == "choice" && ToText(v) != "" {
			have = []string{ToText(v)}
		}
		wants := stringsOf(want)
		if s, ok := want.(string); ok {
			wants = []string{s}
		}
		// a filter may name choices by id or by name
		has := func(w string) bool {
			for _, h := range have {
				if h == w || strings.EqualFold(f.ChoiceName(h), w) {
					return true
				}
			}
			return false
		}
		anyOf := false
		for _, w := range wants {
			if has(w) {
				anyOf = true
				break
			}
		}
		switch fl.Op {
		case "is", "any_of", "contains":
			return anyOf
		case "is_not", "none_of", "not_contains":
			return !anyOf
		case "has_all":
			for _, w := range wants {
				if !has(w) {
					return false
				}
			}
			return true
		}
	}
	w := strings.ToLower(strings.TrimSpace(ToText(want)))
	switch fl.Op {
	case "contains", "any_of":
		return strings.Contains(disp, w)
	case "not_contains", "none_of":
		return !strings.Contains(disp, w)
	case "is":
		return disp == w
	case "is_not":
		return disp != w
	case "gt", "after":
		return disp > w
	case "lt", "before":
		return disp != "" && disp < w
	}
	return true
}

func dateOnly(s string) string {
	if len(s) >= 10 && reDate.MatchString(s[:10]) {
		if strings.HasSuffix(s, "Z") || strings.Contains(s[10:], "+") {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return t.In(time.Local).Format("2006-01-02")
			}
		}
		return s[:10]
	}
	return ""
}

// RelDate reads a date a filter compares with: a day, or today, tomorrow,
// yesterday, or a number of days from today (+7, -30).
func RelDate(s string, now time.Time) string {
	s = strings.ToLower(strings.TrimSpace(s))
	day := func(n int) string { return now.AddDate(0, 0, n).Format("2006-01-02") }
	switch s {
	case "today":
		return day(0)
	case "tomorrow":
		return day(1)
	case "yesterday":
		return day(-1)
	case "":
		return ""
	}
	if (s[0] == '+' || s[0] == '-') && len(s) > 1 {
		if n, ok := ParseNumber(s); ok {
			return day(int(n))
		}
	}
	if d, ok := ToDate(s, false); ok {
		return d
	}
	return ""
}

// within is the span of days for "within": past_week, next_week,
// past_month, next_month, this_week, this_month, this_year, next_7… .
func within(span string, now time.Time) (string, string) {
	d := func(t time.Time) string { return t.Format("2006-01-02") }
	switch span {
	case "past_week":
		return d(now.AddDate(0, 0, -7)), d(now)
	case "next_week":
		return d(now), d(now.AddDate(0, 0, 7))
	case "past_month":
		return d(now.AddDate(0, -1, 0)), d(now)
	case "next_month":
		return d(now), d(now.AddDate(0, 1, 0))
	case "past_year":
		return d(now.AddDate(-1, 0, 0)), d(now)
	case "this_week":
		wd := (int(now.Weekday()) + 6) % 7
		start := now.AddDate(0, 0, -wd)
		return d(start), d(start.AddDate(0, 0, 6))
	case "this_month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return d(start), d(start.AddDate(0, 1, -1))
	case "this_year":
		return now.Format("2006") + "-01-01", now.Format("2006") + "-12-31"
	case "overdue":
		return "0000-01-01", d(now.AddDate(0, 0, -1))
	}
	return d(now), d(now)
}

// Search keeps the records with q in any field.
func (s *Sheet) Search(q string) []*Record {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return s.Records
	}
	var out []*Record
	for _, r := range s.Records {
		for _, f := range s.Fields {
			if strings.Contains(strings.ToLower(s.Text(r, f)), q) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// SortRecords orders records by the sorts in turn; empty values go last.
func (s *Sheet) SortRecords(recs []*Record, sorts []Sort) {
	s.index()
	var keys []Sort
	for _, so := range sorts {
		if s.fields[so.Field] != nil {
			keys = append(keys, so)
		}
	}
	if len(keys) == 0 {
		return
	}
	sort.SliceStable(recs, func(i, j int) bool {
		for _, so := range keys {
			f := s.fields[so.Field]
			c, emptyA, emptyB := cmpValues(f, recs[i].Values[Key(f.ID)], recs[j].Values[Key(f.ID)])
			if emptyA != emptyB {
				return emptyB // empty last, whichever way
			}
			if c == 0 {
				continue
			}
			if so.Desc {
				return c > 0
			}
			return c < 0
		}
		return false
	})
}

func cmpValues(f *Field, a, b any) (int, bool, bool) {
	ea, eb := Display(f, a) == "", Display(f, b) == ""
	if f.Type == Checkbox {
		ea, eb = false, false
	}
	if ea || eb {
		return 0, ea, eb
	}
	switch kind(f, a) {
	case "number":
		x, _ := ToNumber(a)
		y, _ := ToNumber(b)
		switch {
		case x < y:
			return -1, false, false
		case x > y:
			return 1, false, false
		}
		return 0, false, false
	case "bool":
		x, y := ToBool(a), ToBool(b)
		switch {
		case x == y:
			return 0, false, false
		case !x:
			return -1, false, false
		}
		return 1, false, false
	case "choice":
		// in the order the options are listed
		return choiceIndex(f, ToText(a)) - choiceIndex(f, ToText(b)), false, false
	case "date":
		return strings.Compare(ToText(a), ToText(b)), false, false
	}
	return strings.Compare(strings.ToLower(Display(f, a)), strings.ToLower(Display(f, b))), false, false
}

func choiceIndex(f *Field, id string) int {
	for i, c := range f.Options.Choices {
		if c.ID == id {
			return i
		}
	}
	return len(f.Options.Choices)
}

// Summary works out a column's footer: sum, avg, min, max, filled, empty,
// unique, checked, earliest, latest.
func (s *Sheet) Summary(recs []*Record, f *Field, fn string) any {
	var nums []float64
	filled, checked := 0, 0
	uniq := map[string]bool{}
	var dates []string
	for _, r := range recs {
		v := r.Values[Key(f.ID)]
		t := Display(f, v)
		if t != "" {
			filled++
			uniq[t] = true
		}
		if ToBool(v) && f.Type == Checkbox {
			checked++
		}
		if n, ok := v.(float64); ok {
			nums = append(nums, n)
		}
		if d := dateOnly(ToText(v)); d != "" && kind(f, v) == "date" {
			dates = append(dates, ToText(v))
		}
	}
	switch fn {
	case "sum", "avg", "min", "max":
		vals := make([]any, len(nums))
		for i, n := range nums {
			vals[i] = n
		}
		return Roll(fn, vals, len(nums))
	case "filled":
		return float64(filled)
	case "empty":
		return float64(len(recs) - filled)
	case "unique":
		return float64(len(uniq))
	case "checked":
		return float64(checked)
	case "percent_checked":
		if len(recs) == 0 {
			return nil
		}
		return float64(checked) * 100 / float64(len(recs))
	case "earliest", "latest":
		if len(dates) == 0 {
			return nil
		}
		sort.Strings(dates)
		if fn == "earliest" {
			return dates[0]
		}
		return dates[len(dates)-1]
	}
	return nil
}
