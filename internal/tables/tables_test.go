package tables

import (
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	sel := &Field{Name: "Status", Type: Select, Options: Options{Choices: []Choice{{ID: "c1", Name: "Open"}}}}
	cases := []struct {
		f       *Field
		in      any
		lenient bool
		want    any
		bad     bool
	}{
		{&Field{Type: Number}, "1.234,50", false, 1234.5, false},
		{&Field{Type: Number}, "1,234.50", false, 1234.5, false},
		{&Field{Type: Number}, "12,5", false, 12.5, false},
		{&Field{Type: Number}, "abc", false, nil, true},
		{&Field{Type: Number}, "abc", true, nil, false},
		{&Field{Type: Checkbox}, "da", false, true, false},
		{&Field{Type: Date}, "7.11.2026", false, "2026-11-07", false},
		{&Field{Type: Date, Options: Options{Time: true}}, "2026-11-07 09:30", false, "2026-11-07T09:30", false},
		{&Field{Type: Email}, "Ana <ANA@Pekara.ba>", false, "ana@pekara.ba", false},
		{&Field{Type: URL}, "example.com", false, "https://example.com", false},
		{&Field{Type: Rating, Options: Options{Max: 5}}, 9, false, 5.0, false},
		{sel, "open", false, "c1", false},
		{sel, "Closed", false, nil, true},
	}
	for i, c := range cases {
		c.f.Name = "F"
		got, err := Normalize(c.f, c.in, c.lenient)
		if (err != nil) != c.bad {
			t.Errorf("%d: err %v", i, err)
			continue
		}
		if !c.bad && got != c.want {
			t.Errorf("%d: got %#v, want %#v", i, got, c.want)
		}
	}
	got, _ := Normalize(sel, "Closed", true)
	if got != "c2" || len(sel.Options.Choices) != 2 {
		t.Errorf("lenient select: %v %v", got, sel.Options.Choices)
	}
}

func TestFormula(t *testing.T) {
	fields := []*Field{{ID: 1, Name: "Price", Type: Number}, {ID: 2, Name: "Qty", Type: Number}, {ID: 3, Name: "Due", Type: Date},
		{ID: 4, Name: "Name", Type: Text}, {ID: 5, Name: "Paid", Type: Checkbox}}
	s := &Sheet{Fields: fields}
	vals := map[int64]any{1: 2.5, 2: 4.0, 3: "2026-11-10", 4: "ana", 5: true}
	env := &Env{Get: func(id int64) any { return vals[id] }, Now: time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local)}
	for src, want := range map[string]any{
		"{Price} * {Qty}":                            10.0,
		"ROUND({Price} / 3, 2)":                      0.83,
		`IF({Paid}, "done", "open")`:                 "done",
		`UPPER({Name}) & "!"`:                        "ANA!",
		"DAYS({Due}, TODAY())":                       3.0,
		`DATEADD({Due}, 1, "months")`:                "2026-12-10",
		"{Price} > 2 && {Qty} = 4":                   true,
		"SUM({Price}, {Qty}, 1)":                     7.5,
		`SWITCH({Qty}, 1, "one", 4, "four", "many")`: "four",
		"Price + 1":                                  3.5,
		"1 / 0":                                      nil,
		`LEN(CONCAT({Name}, "  "))`:                  5.0,
	} {
		e, _, err := Parse(src, s.Resolver())
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if got := Eval(e, env); got != want {
			t.Errorf("%s = %#v, want %#v", src, got, want)
		}
	}
	for _, bad := range []string{"{Nope}", "NOPE(1)", "IF(1", `"open`, "1 +"} {
		if _, _, err := Parse(bad, s.Resolver()); err == nil {
			t.Errorf("%s parsed", bad)
		}
	}
	if got := RenameRefs(`{Price} * {price} + {Qty}`, "Price", "Cost"); got != `{Cost} * {Cost} + {Qty}` {
		t.Errorf("rename: %s", got)
	}
}

func TestInfer(t *testing.T) {
	for _, c := range []struct {
		name string
		vals []string
		want string
	}{
		{"Email", []string{"a@b.ba", "c@d.ba"}, Email},
		{"Born", []string{"1990-02-01", "12.3.1988"}, Date},
		{"Count", []string{"1", "22", "3.5"}, Number},
		{"Price", []string{"€ 12", "€3,50"}, Currency},
		{"Done", []string{"yes", "no", "yes"}, Checkbox},
		{"Team", []string{"Red", "Blue", "Red", "Red", "Blue"}, Select},
		{"Name", []string{"Ana", "Emir", "Lejla"}, Text},
		{"Site", []string{"https://a.ba"}, URL},
	} {
		if got, _ := Infer(c.name, c.vals); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFilterAndSort(t *testing.T) {
	fields := []*Field{{ID: 1, Name: "Name", Type: Text, Primary: true}, {ID: 2, Name: "Pages", Type: Number}, {ID: 3, Name: "Read", Type: Date},
		{ID: 4, Name: "Kind", Type: Select, Options: Options{Choices: []Choice{{ID: "a", Name: "Novel"}, {ID: "b", Name: "Poems"}}}}}
	rec := func(id int64, name string, pages float64, read, kind string) *Record {
		d := map[string]any{"1": name, "2": pages, "4": kind}
		if read != "" {
			d["3"] = read
		}
		return &Record{ID: id, Data: d}
	}
	s := &Sheet{Fields: fields, Records: []*Record{rec(1, "Dune", 612, "2026-11-01", "a"), rec(2, "Emma", 474, "", "a"), rec(3, "Odes", 90, "2026-11-06", "b")}}
	s.Compute()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local)
	count := func(fl []Filter, any bool) int {
		n := 0
		for _, r := range s.Records {
			if s.Match(r, fl, any, now) {
				n++
			}
		}
		return n
	}
	if n := count([]Filter{{Field: 2, Op: "gt", Value: 400}}, false); n != 2 {
		t.Errorf("gt: %d", n)
	}
	if n := count([]Filter{{Field: 3, Op: "within", Value: "past_week"}}, false); n != 2 {
		t.Errorf("within: %d", n)
	}
	if n := count([]Filter{{Field: 4, Op: "is", Value: "Poems"}}, false); n != 1 {
		t.Errorf("choice by name: %d", n)
	}
	if n := count([]Filter{{Field: 3, Op: "empty"}, {Field: 4, Op: "is", Value: "b"}}, true); n != 2 {
		t.Errorf("any: %d", n)
	}
	recs := append([]*Record(nil), s.Records...)
	s.SortRecords(recs, []Sort{{Field: 3, Desc: true}})
	if recs[0].ID != 3 || recs[2].ID != 2 {
		t.Errorf("sort with empty last: %d %d %d", recs[0].ID, recs[1].ID, recs[2].ID)
	}
	if got := s.Summary(s.Records, fields[1], "avg"); got != 392.0 {
		t.Errorf("avg: %v", got)
	}
}
