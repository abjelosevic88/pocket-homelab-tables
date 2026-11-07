// Package tables is the engine behind Pocket Tables: field types and what a
// value of each looks like, formulas, filters and sorting, and working out
// the values that come from other fields (links, lookups, rollups). It knows
// nothing about HTTP or SQL, so all of it can be tested on its own.
package tables

import (
	"fmt"
	"math"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Field types.
const (
	Text       = "text"
	LongText   = "longtext"
	Number     = "number"
	Currency   = "currency"
	Percent    = "percent"
	Rating     = "rating"
	Checkbox   = "checkbox"
	Date       = "date"
	Select     = "select"
	Multi      = "multiselect"
	Email      = "email"
	Phone      = "phone"
	URL        = "url"
	Link       = "link"
	Lookup     = "lookup"
	Rollup     = "rollup"
	Formula    = "formula"
	Attachment = "attachment"
	Pocket     = "pocket"
	Created    = "created"
	Updated    = "updated"
	AutoNumber = "autonumber"
)

// Types lists every field type in the order the "new field" menu shows them.
var Types = []string{Text, LongText, Number, Currency, Percent, Rating, Checkbox, Date, Select, Multi, Email, Phone, URL, Attachment, Link, Lookup, Rollup, Formula, Pocket, Created, Updated, AutoNumber}

// ValidType reports whether t is a field type.
func ValidType(t string) bool {
	for _, x := range Types {
		if x == t {
			return true
		}
	}
	return false
}

// Computed types are worked out, never typed in.
func Computed(t string) bool {
	switch t {
	case Lookup, Rollup, Formula, Created, Updated, AutoNumber:
		return true
	}
	return false
}

// Numeric types hold a number.
func Numeric(t string) bool {
	switch t {
	case Number, Currency, Percent, Rating, AutoNumber:
		return true
	}
	return false
}

// Choice is one option of a select field.
type Choice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Options are the settings of a field; each type reads only its own.
type Options struct {
	Decimals  int      `json:"decimals,omitempty"`   // number, currency, percent
	Currency  string   `json:"currency,omitempty"`   // currency
	Max       int      `json:"max,omitempty"`        // rating (default 5)
	Time      bool     `json:"time,omitempty"`       // date: with a time of day
	Remind    bool     `json:"remind,omitempty"`     // date: a morning message on the day
	Choices   []Choice `json:"choices,omitempty"`    // select, multiselect
	TableID   int64    `json:"table_id,omitempty"`   // link: the other table
	Inverse   int64    `json:"inverse,omitempty"`    // link: the field on the other side
	Reverse   bool     `json:"reverse,omitempty"`    // link: this is the other side (links are stored under Inverse)
	Single    bool     `json:"single,omitempty"`     // link, pocket: only one
	LinkField int64    `json:"link_field,omitempty"` // lookup, rollup: through this link
	Field     int64    `json:"field,omitempty"`      // lookup, rollup: this field of the linked records
	Fn        string   `json:"fn,omitempty"`         // rollup: sum avg min max count counta checked list earliest latest
	Expr      string   `json:"expr,omitempty"`       // formula
	App       string   `json:"app,omitempty"`        // pocket: which kind of thing (contact, task, event…)
}

// Field is one column of a table.
type Field struct {
	ID          int64   `json:"id"`
	TableID     int64   `json:"table_id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Options     Options `json:"options"`
	Description string  `json:"description"`
	Position    float64 `json:"position"`
	Primary     bool    `json:"primary"`
	Required    bool    `json:"required"`
}

// RollupFns are what a rollup can do with the linked values.
var RollupFns = []string{"sum", "avg", "min", "max", "count", "counta", "checked", "list", "earliest", "latest"}

// Colors are offered for new choices, in turn.
var Colors = []string{"#4dabf7", "#69db7c", "#ffd43b", "#ff8787", "#b197fc", "#ffa94d", "#63e6be", "#f783ac", "#a9e34b", "#74c0fc", "#ced4da", "#e599f7"}

// PocketRef is a thing in another Pocket app a cell points at.
type PocketRef struct {
	App   string `json:"app"`   // contact | task | event | note | document | item | place | deal | book | wish
	Ref   string `json:"ref"`   // its id there
	Title string `json:"title"` // as it was when picked
	Sub   string `json:"sub,omitempty"`
	URL   string `json:"url,omitempty"`
}

// File is one attached file.
type File struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

// LinkRef is a linked record as a cell shows it.
type LinkRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

var (
	reDate      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reDateTime  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}`)
	reNumClean  = regexp.MustCompile(`[^\d,.\-+eE]`)
	rePhone     = regexp.MustCompile(`^[+()\d][\d\s()./-]{2,30}$`)
	reThousands = regexp.MustCompile(`^-?\d{1,3}([.,]\d{3})+$`)
	reDigits    = regexp.MustCompile(`^-?\d+$`)
)

// InputError is a value a person typed that does not fit the field.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func bad(f *Field, what string) error {
	return &InputError{fmt.Sprintf("%s: %s", f.Name, what)}
}

// Normalize turns what came in (from the page, a CSV, another app or AI)
// into the stored value of a field: nil when empty. With lenient set, a
// select takes unknown names as new choices (they are added to f) and a
// value that does not fit is dropped instead of refused.
func Normalize(f *Field, in any, lenient bool) (any, error) {
	if in == nil {
		return nil, nil
	}
	if s, ok := in.(string); ok && strings.TrimSpace(s) == "" && f.Type != Checkbox {
		return nil, nil
	}
	fail := func(what string) (any, error) {
		if lenient {
			return nil, nil
		}
		return nil, bad(f, what)
	}
	switch f.Type {
	case Text, Phone:
		s := strings.TrimSpace(ToText(in))
		if f.Type == Text {
			s = strings.ReplaceAll(s, "\n", " ")
		}
		if f.Type == Phone && !rePhone.MatchString(s) {
			return fail("that is not a phone number")
		}
		return clip(s, 10000), nil
	case LongText:
		return clip(strings.TrimRight(ToText(in), " \n"), 100000), nil
	case Email:
		s := strings.ToLower(strings.TrimSpace(ToText(in)))
		if a, err := mail.ParseAddress(s); err == nil {
			s = strings.ToLower(a.Address)
		} else {
			return fail("that is not an email address")
		}
		return s, nil
	case URL:
		s := strings.TrimSpace(ToText(in))
		if !strings.Contains(s, "://") && !strings.HasPrefix(s, "mailto:") {
			if !strings.Contains(s, ".") || strings.ContainsAny(s, " \t") {
				return fail("that is not a web address")
			}
			s = "https://" + s
		}
		return clip(s, 4000), nil
	case Number, Currency, Percent, Rating:
		n, ok := ToNumber(in)
		if !ok {
			return fail("that is not a number")
		}
		if f.Type == Rating {
			n = math.Round(n)
			mx := float64(f.Options.Max)
			if mx <= 0 {
				mx = 5
			}
			if n <= 0 {
				return nil, nil
			}
			n = math.Min(n, mx)
		}
		return n, nil
	case Checkbox:
		return ToBool(in), nil
	case Date:
		d, ok := ToDate(ToText(in), f.Options.Time)
		if !ok {
			return fail("that is not a date (YYYY-MM-DD)")
		}
		return d, nil
	case Select:
		name := in
		if arr, ok := in.([]any); ok {
			if len(arr) == 0 {
				return nil, nil
			}
			name = arr[0]
		}
		c := f.choice(ToText(name), lenient)
		if c == nil {
			return fail("there is no option " + strconv.Quote(ToText(name)))
		}
		return c.ID, nil
	case Multi:
		var parts []string
		switch t := in.(type) {
		case []any:
			for _, x := range t {
				parts = append(parts, ToText(x))
			}
		case []string:
			parts = t
		default:
			parts = splitList(ToText(in))
		}
		out := []string{}
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				continue
			}
			c := f.choice(p, lenient)
			if c == nil {
				if lenient {
					continue
				}
				return nil, bad(f, "there is no option "+strconv.Quote(p))
			}
			if !contains(out, c.ID) {
				out = append(out, c.ID)
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case Attachment:
		files := toFiles(in)
		if len(files) == 0 {
			return nil, nil
		}
		return files, nil
	case Pocket:
		refs := toRefs(in, f.Options.App)
		if len(refs) == 0 {
			return nil, nil
		}
		if f.Options.Single && len(refs) > 1 {
			refs = refs[:1]
		}
		return refs, nil
	}
	return nil, nil // computed and link fields are not stored in a record
}

// choice finds a select option by id or name (case-insensitively); lenient
// adds an unknown name as a new option.
func (f *Field) choice(v string, lenient bool) *Choice {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	for i := range f.Options.Choices {
		if f.Options.Choices[i].ID == v {
			return &f.Options.Choices[i]
		}
	}
	for i := range f.Options.Choices {
		if strings.EqualFold(f.Options.Choices[i].Name, v) {
			return &f.Options.Choices[i]
		}
	}
	if !lenient || len(f.Options.Choices) >= 200 {
		return nil
	}
	c := Choice{ID: NewChoiceID(f.Options.Choices), Name: clip(v, 200), Color: Colors[len(f.Options.Choices)%len(Colors)]}
	f.Options.Choices = append(f.Options.Choices, c)
	return &f.Options.Choices[len(f.Options.Choices)-1]
}

// ChoiceName is the name of a select option ("" when unknown).
func (f *Field) ChoiceName(id string) string {
	for _, c := range f.Options.Choices {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

// NewChoiceID is an id not used by any of the choices.
func NewChoiceID(cs []Choice) string {
	used := map[string]bool{}
	for _, c := range cs {
		used[c.ID] = true
	}
	for i := len(cs) + 1; ; i++ {
		id := "c" + strconv.Itoa(i)
		if !used[id] {
			return id
		}
	}
}

// CleanChoices gives every choice an id and a colour and drops empty and repeated names.
func CleanChoices(in []Choice) []Choice {
	out := []Choice{}
	seen := map[string]bool{}
	given := map[string]bool{}
	for _, c := range in {
		if c.ID != "" {
			given[c.ID] = true
		}
	}
	used := map[string]bool{}
	next := 1
	for _, c := range in {
		c.Name = clip(strings.TrimSpace(c.Name), 200)
		if c.Name == "" || seen[strings.ToLower(c.Name)] {
			continue
		}
		seen[strings.ToLower(c.Name)] = true
		if c.ID == "" || used[c.ID] {
			for c.ID = "c" + strconv.Itoa(next); given[c.ID] || used[c.ID]; c.ID = "c" + strconv.Itoa(next) {
				next++
			}
		}
		used[c.ID] = true
		if c.Color == "" {
			c.Color = Colors[len(out)%len(Colors)]
		}
		out = append(out, c)
	}
	return out
}

func toFiles(in any) []File {
	arr, ok := in.([]any)
	if !ok {
		return nil
	}
	out := []File{}
	for _, x := range arr {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		id := ToText(m["id"])
		if id == "" {
			continue
		}
		n, _ := ToNumber(m["size"])
		out = append(out, File{ID: id, Name: clip(ToText(m["name"]), 300), Mime: clip(ToText(m["mime"]), 120), Size: int64(n)})
	}
	return out
}

func toRefs(in any, app string) []PocketRef {
	var arr []any
	switch t := in.(type) {
	case []any:
		arr = t
	case map[string]any:
		arr = []any{t}
	case []PocketRef:
		return t
	default:
		return nil
	}
	out := []PocketRef{}
	for _, x := range arr {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		r := PocketRef{App: ToText(m["app"]), Ref: ToText(m["ref"]), Title: clip(ToText(m["title"]), 500), Sub: clip(ToText(m["sub"]), 500), URL: clip(ToText(m["url"]), 2000)}
		if r.App == "" {
			r.App = app
		}
		if r.Title == "" {
			r.Title = r.Ref
		}
		if r.Ref == "" && r.Title == "" {
			continue
		}
		if app != "" && r.App != app {
			continue
		}
		if r.URL != "" && !strings.HasPrefix(r.URL, "http://") && !strings.HasPrefix(r.URL, "https://") {
			r.URL = ""
		}
		out = append(out, r)
	}
	return out
}

// ToText is any value as plain text.
func ToText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return FormatNumber(t, -1)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			if s := ToText(x); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(t, ", ")
	case map[string]any:
		for _, k := range []string{"title", "name", "label", "value", "id"} {
			if s := ToText(t[k]); s != "" {
				return s
			}
		}
		return ""
	}
	return fmt.Sprint(v)
}

// ToNumber reads a number from text too: "1.234,50", "€ 12", "45%".
func ToNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, !math.IsNaN(t) && !math.IsInf(t, 0)
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case string:
		return ParseNumber(t)
	case []any:
		if len(t) == 1 {
			return ToNumber(t[0])
		}
	}
	return 0, false
}

// ParseNumber understands both 1,234.5 and 1.234,5.
func ParseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")
	s = reNumClean.ReplaceAllString(s, "")
	if s == "" || s == "-" || s == "+" {
		return 0, false
	}
	dots, commas := strings.Count(s, "."), strings.Count(s, ",")
	switch {
	case dots > 0 && commas > 0:
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case commas == 1 && dots == 0:
		if i := strings.Index(s, ","); len(s)-i-1 == 3 && i > 0 && i <= 3 && !strings.HasPrefix(s, "0") {
			s = strings.ReplaceAll(s, ",", "") // 1,234
		} else {
			s = strings.ReplaceAll(s, ",", ".") // 12,5
		}
	case commas > 1:
		s = strings.ReplaceAll(s, ",", "")
	case dots > 1:
		s = strings.ReplaceAll(s, ".", "")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}

// ToBool reads yes/no in a few languages.
func ToBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "true", "yes", "y", "x", "✓", "✔", "checked", "da", "ja", "on", "done", "ok":
			return true
		}
	}
	return false
}

var dateLayouts = []string{"2006-01-02", "2.1.2006", "02.01.2006", "2.1.2006.", "02.01.2006.", "1/2/2006", "01/02/2006", "2006/01/02", "2 Jan 2006", "Jan 2, 2006", "January 2, 2006", "2 January 2006", "02-01-2006"}
var dateTimeLayouts = []string{"2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2.1.2006 15:04", "02.01.2006 15:04", "1/2/2006 15:04", "1/2/2006 3:04 PM", "01/02/2006 15:04"}

// ToDate reads a date (and with withTime a time of day) as YYYY-MM-DD or
// YYYY-MM-DDTHH:MM in local time.
func ToDate(s string, withTime bool) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		t = t.In(time.Local)
		if withTime {
			return t.Format("2006-01-02T15:04"), true
		}
		return t.Format("2006-01-02"), true
	}
	for _, l := range dateTimeLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			if withTime {
				return t.Format("2006-01-02T15:04"), true
			}
			return t.Format("2006-01-02"), true
		}
	}
	for _, l := range dateLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			if t.Year() < 1 || t.Year() > 9999 {
				return "", false
			}
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// FormatNumber prints a number with this many decimals (-1 = as needed).
func FormatNumber(n float64, decimals int) string {
	if decimals < 0 {
		if n == math.Trunc(n) && math.Abs(n) < 1e15 {
			return strconv.FormatFloat(n, 'f', 0, 64)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return strconv.FormatFloat(n, 'f', decimals, 64)
}

// Display is a cell as plain text: for search, CSV, notes, AI and the other apps.
func Display(f *Field, v any) string {
	if v == nil {
		return ""
	}
	switch f.Type {
	case Number:
		if n, ok := ToNumber(v); ok {
			return FormatNumber(n, decimals(f, -1))
		}
	case Currency:
		if n, ok := ToNumber(v); ok {
			s := FormatNumber(n, decimals(f, 2))
			if f.Options.Currency != "" {
				s += " " + f.Options.Currency
			}
			return s
		}
	case Percent:
		if n, ok := ToNumber(v); ok {
			return FormatNumber(n, decimals(f, -1)) + "%"
		}
	case Rating:
		if n, ok := ToNumber(v); ok {
			return strings.Repeat("★", int(n))
		}
	case Checkbox:
		if ToBool(v) {
			return "yes"
		}
		return ""
	case Select:
		return f.ChoiceName(ToText(v))
	case Multi:
		var names []string
		for _, id := range stringsOf(v) {
			if n := f.ChoiceName(id); n != "" {
				names = append(names, n)
			}
		}
		return strings.Join(names, ", ")
	case Attachment:
		var names []string
		for _, x := range toFiles(v) {
			names = append(names, x.Name)
		}
		return strings.Join(names, ", ")
	case Pocket:
		var names []string
		for _, x := range toRefs(v, "") {
			names = append(names, x.Title)
		}
		return strings.Join(names, ", ")
	case Link:
		var names []string
		for _, x := range linkRefs(v) {
			names = append(names, x.Title)
		}
		return strings.Join(names, ", ")
	case Created, Updated:
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return t.In(time.Local).Format("2006-01-02 15:04")
			}
		}
	}
	if arr, ok := v.([]any); ok {
		parts := make([]string, 0, len(arr))
		for _, x := range arr {
			if s := ToText(x); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	if n, ok := v.(float64); ok && (f.Type == Formula || f.Type == Rollup || f.Type == Lookup) {
		return FormatNumber(math.Round(n*1e6)/1e6, decimals(f, -1))
	}
	return ToText(v)
}

func decimals(f *Field, def int) int {
	if f.Options.Decimals > 0 {
		return min(f.Options.Decimals, 8)
	}
	return def
}

func linkRefs(v any) []LinkRef {
	switch t := v.(type) {
	case []LinkRef:
		return t
	case []any:
		out := []LinkRef{}
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				n, _ := ToNumber(m["id"])
				out = append(out, LinkRef{ID: int64(n), Title: ToText(m["title"])})
			}
		}
		return out
	}
	return nil
}

func stringsOf(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, ToText(x))
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	}
	return nil
}

// splitList splits "a, b; c" (or one per line).
func splitList(s string) []string {
	f := func(r rune) bool { return r == ',' || r == ';' || r == '\n' }
	parts := strings.FieldsFunc(s, f)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Infer guesses the type of a column from its values (a CSV import, a list
// from another app): with the choices for a select when few values repeat.
func Infer(name string, values []string) (string, Options) {
	var filled []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			filled = append(filled, v)
		}
	}
	if len(filled) == 0 {
		return Text, Options{}
	}
	all := func(ok func(string) bool) bool {
		for _, v := range filled {
			if !ok(v) {
				return false
			}
		}
		return true
	}
	lname := strings.ToLower(name)
	isBool := func(v string) bool {
		switch strings.ToLower(v) {
		case "true", "false", "yes", "no", "y", "n", "x", "✓", "✔", "checked", "unchecked", "da", "ne", "ja", "nein":
			return true
		}
		return false
	}
	switch {
	case all(isBool) && len(filled) >= 1:
		return Checkbox, Options{}
	case all(func(v string) bool {
		_, err := mail.ParseAddress(v)
		return err == nil && strings.Contains(v, "@") && !strings.Contains(v, " ")
	}):
		return Email, Options{}
	case all(func(v string) bool { return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") }):
		return URL, Options{}
	case all(func(v string) bool { return reDateTime.MatchString(v) && len(v) > 10 }):
		if all(func(v string) bool { return strings.HasSuffix(v, "00:00") || strings.HasSuffix(v, "00:00:00") }) {
			return Date, Options{}
		}
		return Date, Options{Time: true}
	case all(func(v string) bool {
		_, ok := ToDate(v, false)
		return ok && !reThousands.MatchString(v) && !reDigits.MatchString(v)
	}):
		return Date, Options{}
	case all(func(v string) bool { return strings.HasSuffix(v, "%") && isPlainNumber(strings.TrimSuffix(v, "%")) }):
		return Percent, Options{}
	case all(func(v string) bool { c, ok := currencyOf(v); return ok && c != "" }):
		c, _ := currencyOf(filled[0])
		return Currency, Options{Currency: c, Decimals: 2}
	case all(isPlainNumber):
		if strings.Contains(lname, "price") || strings.Contains(lname, "cost") || strings.Contains(lname, "amount") || strings.Contains(lname, "cijena") || strings.Contains(lname, "iznos") {
			return Currency, Options{Decimals: 2}
		}
		if (strings.Contains(lname, "phone") || strings.Contains(lname, "tel")) && !strings.Contains(lname, "total") {
			return Phone, Options{}
		}
		return Number, Options{Decimals: maxDecimals(filled)}
	case strings.Contains(lname, "phone") || strings.Contains(lname, "mobile") || strings.Contains(lname, "telefon"):
		if all(func(v string) bool { return rePhone.MatchString(v) }) {
			return Phone, Options{}
		}
	}
	// a few values repeated many times: options to pick from
	counts := map[string]int{}
	var order []string
	long := false
	multi := true
	for _, v := range filled {
		if len(v) > 60 {
			long = true
		}
		if !strings.Contains(v, ",") {
			multi = false
		}
		if counts[v] == 0 {
			order = append(order, v)
		}
		counts[v]++
	}
	for _, v := range filled {
		if strings.Contains(v, "\n") || len(v) > 250 {
			return LongText, Options{}
		}
	}
	if !long && len(filled) >= 4 && len(order) <= 12 && len(order)*2 <= len(filled) {
		if multi {
			seen := map[string]bool{}
			var names []string
			for _, v := range filled {
				for _, p := range splitList(v) {
					if !seen[strings.ToLower(p)] {
						seen[strings.ToLower(p)] = true
						names = append(names, p)
					}
				}
			}
			if len(names) <= 25 {
				return Multi, Options{Choices: choicesOf(names)}
			}
		}
		sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
		return Select, Options{Choices: choicesOf(order)}
	}
	return Text, Options{}
}

func choicesOf(names []string) []Choice {
	cs := make([]Choice, 0, len(names))
	for i, n := range names {
		cs = append(cs, Choice{ID: "c" + strconv.Itoa(i+1), Name: n, Color: Colors[i%len(Colors)]})
	}
	return cs
}

func isPlainNumber(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	for _, r := range v {
		if !strings.ContainsRune("0123456789.,-+ ", r) {
			return false
		}
	}
	_, ok := ParseNumber(v)
	return ok
}

var currencySymbols = map[string]string{"€": "EUR", "$": "USD", "£": "GBP", "KM": "BAM", "kn": "HRK", "din": "RSD", "CHF": "CHF", "EUR": "EUR", "USD": "USD", "BAM": "BAM", "GBP": "GBP", "RSD": "RSD"}

func currencyOf(v string) (string, bool) {
	v = strings.TrimSpace(v)
	for sym, code := range currencySymbols {
		if strings.HasPrefix(v, sym) || strings.HasSuffix(v, sym) {
			rest := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(v, sym), sym))
			if isPlainNumber(rest) {
				return code, true
			}
		}
	}
	return "", false
}

func maxDecimals(vals []string) int {
	d := 0
	for _, v := range vals {
		n, ok := ParseNumber(v)
		if !ok {
			continue
		}
		s := strconv.FormatFloat(n, 'f', -1, 64)
		if i := strings.Index(s, "."); i >= 0 {
			d = max(d, len(s)-i-1)
		}
	}
	return min(d, 4)
}
