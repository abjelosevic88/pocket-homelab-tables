package tables

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ---- formulas ----
//
// A spreadsheet-like language people already know: {Field name} for a
// field, numbers, "text", + - * / % & (join text), = != < > <= >= and
// functions like IF, SUM, ROUND, CONCAT, TODAY, DATEADD, DAYS. Names of
// functions are not case-sensitive. A formula is checked when it is saved
// (unknown fields and functions are refused) and worked out when records
// are read.

// Expr is a parsed formula.
type Expr interface{}

type numLit struct{ v float64 }
type strLit struct{ v string }
type boolLit struct{ v bool }
type fieldRef struct {
	id   int64
	name string
}
type unary struct {
	op string
	x  Expr
}
type binary struct {
	op   string
	l, r Expr
}
type call struct {
	fn   string
	args []Expr
}

type token struct {
	kind string // num str ident field op ( ) , eof
	s    string
	pos  int
}

func lex(src string) ([]token, error) {
	var out []token
	rs := []rune(src)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '{':
			j := i + 1
			for j < len(rs) && rs[j] != '}' {
				j++
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("a field name is not closed with }")
			}
			out = append(out, token{"field", strings.TrimSpace(string(rs[i+1 : j])), i})
			i = j + 1
		case c == '"' || c == '\'' || c == '“' || c == '”':
			end := c
			if c == '“' {
				end = '”'
			}
			var b strings.Builder
			j := i + 1
			for ; j < len(rs) && rs[j] != end; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				}
				b.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("a text is not closed with %c", end)
			}
			out = append(out, token{"str", b.String(), i})
			i = j + 1
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(rs) && unicode.IsDigit(rs[i+1])):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'e' || rs[j] == 'E') {
				j++
			}
			out = append(out, token{"num", string(rs[i:j]), i})
			i = j
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			out = append(out, token{"ident", string(rs[i:j]), i})
			i = j
		default:
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "<=", ">=", "!=", "<>", "==", "&&", "||":
				out = append(out, token{"op", two, i})
				i += 2
				continue
			}
			switch c {
			case '+', '-', '*', '/', '%', '&', '=', '<', '>', '!', '^':
				out = append(out, token{"op", string(c), i})
			case '(', ')', ',', ';':
				k := string(c)
				if c == ';' {
					k = ","
				}
				out = append(out, token{k, k, i})
			default:
				return nil, fmt.Errorf("unexpected %q", string(c))
			}
			i++
		}
	}
	return append(out, token{"eof", "", len(rs)}), nil
}

type parser struct {
	toks    []token
	i       int
	resolve func(name string) (int64, bool)
	refs    []int64
}

// Parse checks a formula; resolve maps a field name to its id.
func Parse(src string, resolve func(name string) (int64, bool)) (Expr, []int64, error) {
	src = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(src), "="))
	if src == "" {
		return nil, nil, fmt.Errorf("the formula is empty")
	}
	toks, err := lex(src)
	if err != nil {
		return nil, nil, err
	}
	p := &parser{toks: toks, resolve: resolve}
	e, err := p.expr(0)
	if err != nil {
		return nil, nil, err
	}
	if p.peek().kind != "eof" {
		return nil, nil, fmt.Errorf("unexpected %q", p.peek().s)
	}
	return e, p.refs, nil
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token {
	t := p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

var prec = map[string]int{"||": 1, "&&": 2, "=": 3, "==": 3, "!=": 3, "<>": 3, "<": 4, ">": 4, "<=": 4, ">=": 4, "&": 5, "+": 6, "-": 6, "*": 7, "/": 7, "%": 7, "^": 8}

func (p *parser) expr(min int) (Expr, error) {
	left, err := p.atom()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != "op" {
			return left, nil
		}
		pr, ok := prec[t.s]
		if !ok || pr <= min {
			return left, nil
		}
		p.next()
		next := pr
		if t.s == "^" {
			next = pr - 1 // right-associative
		}
		right, err := p.expr(next)
		if err != nil {
			return nil, err
		}
		left = binary{op: t.s, l: left, r: right}
	}
}

func (p *parser) atom() (Expr, error) {
	t := p.next()
	switch t.kind {
	case "num":
		n, err := strconv.ParseFloat(t.s, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", t.s)
		}
		return numLit{n}, nil
	case "str":
		return strLit{t.s}, nil
	case "field":
		id, ok := p.resolve(t.s)
		if !ok {
			return nil, fmt.Errorf("there is no field {%s}", t.s)
		}
		p.refs = append(p.refs, id)
		return fieldRef{id, t.s}, nil
	case "op":
		if t.s == "-" || t.s == "+" || t.s == "!" {
			x, err := p.expr(7)
			if err != nil {
				return nil, err
			}
			return unary{t.s, x}, nil
		}
	case "(":
		e, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		if p.next().kind != ")" {
			return nil, fmt.Errorf("a ( is not closed")
		}
		return e, nil
	case "ident":
		name := strings.ToUpper(t.s)
		switch name {
		case "TRUE":
			if p.peek().kind != "(" {
				return boolLit{true}, nil
			}
		case "FALSE":
			if p.peek().kind != "(" {
				return boolLit{false}, nil
			}
		}
		if p.peek().kind != "(" {
			// a bare field name without braces, when it is one
			if id, ok := p.resolve(t.s); ok {
				p.refs = append(p.refs, id)
				return fieldRef{id, t.s}, nil
			}
			return nil, fmt.Errorf("unknown name %q (put field names in braces: {%s})", t.s, t.s)
		}
		if _, ok := funcs[name]; !ok {
			return nil, fmt.Errorf("there is no function %s()", name)
		}
		p.next()
		var args []Expr
		if p.peek().kind != ")" {
			for {
				a, err := p.expr(0)
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if p.peek().kind == "," {
					p.next()
					continue
				}
				break
			}
		}
		if p.next().kind != ")" {
			return nil, fmt.Errorf("%s( is not closed", name)
		}
		if a := funcs[name]; len(args) < a.min || (a.max >= 0 && len(args) > a.max) {
			return nil, fmt.Errorf("%s() takes %s", name, a.args)
		}
		return call{name, args}, nil
	case "eof":
		return nil, fmt.Errorf("the formula ends too early")
	}
	return nil, fmt.Errorf("unexpected %q", t.s)
}

// Env gives a formula the values of the record's fields.
type Env struct {
	Get func(id int64) any
	Now time.Time
}

type fnInfo struct {
	min, max int
	args     string
	help     string
}

// funcs are the functions a formula may call, with what they take (for the help sheet).
var funcs = map[string]fnInfo{
	"IF": {2, 3, "a condition, the value when true and (optional) when false", "IF({Paid}, \"Done\", \"Waiting\")"}, "AND": {1, -1, "conditions", "AND({Paid}, {Sent})"},
	"OR": {1, -1, "conditions", "OR({Late}, {Urgent})"}, "NOT": {1, 1, "one condition", "NOT({Done})"},
	"SWITCH": {3, -1, "a value, then pairs of case and result, and a default", "SWITCH({Size}, \"S\", 1, \"M\", 2, 0)"},
	"SUM":    {1, -1, "numbers", "SUM({Price}, {Shipping})"}, "AVERAGE": {1, -1, "numbers", "AVERAGE({Q1}, {Q2})"}, "AVG": {1, -1, "numbers", ""},
	"MIN": {1, -1, "numbers", ""}, "MAX": {1, -1, "numbers", ""}, "COUNT": {1, -1, "values", "COUNT({Links})"},
	"ROUND": {1, 2, "a number and how many decimals", "ROUND({Price} * 1.17, 2)"}, "ROUNDUP": {1, 2, "a number and decimals", ""}, "ROUNDDOWN": {1, 2, "a number and decimals", ""},
	"ABS": {1, 1, "a number", ""}, "SQRT": {1, 1, "a number", ""}, "POWER": {2, 2, "a number and the power", ""}, "MOD": {2, 2, "two numbers", ""},
	"VALUE": {1, 1, "text", "VALUE(\"12,5\")"}, "CONCAT": {1, -1, "texts", "CONCAT({First}, \" \", {Last})"},
	"UPPER": {1, 1, "text", ""}, "LOWER": {1, 1, "text", ""}, "TRIM": {1, 1, "text", ""}, "LEN": {1, 1, "text", ""},
	"LEFT": {2, 2, "text and a count", ""}, "RIGHT": {2, 2, "text and a count", ""}, "MID": {3, 3, "text, where to start and a count", ""},
	"FIND": {2, 2, "what to find and in which text", ""}, "CONTAINS": {2, 2, "a text and what to look for", "CONTAINS({Notes}, \"urgent\")"},
	"SUBSTITUTE": {3, 3, "text, what and with what", ""}, "REPT": {2, 2, "text and how many times", ""},
	"BLANK": {0, 0, "nothing", ""}, "ISBLANK": {1, 1, "a value", "ISBLANK({Email})"}, "IFBLANK": {2, 2, "a value and what to show instead", ""},
	"TODAY": {0, 0, "nothing", "DAYS({Due}, TODAY())"}, "NOW": {0, 0, "nothing", ""},
	"YEAR": {1, 1, "a date", ""}, "MONTH": {1, 1, "a date", ""}, "DAY": {1, 1, "a date", ""}, "WEEKDAY": {1, 1, "a date (1 = Monday)", ""}, "WEEKNUM": {1, 1, "a date", ""},
	"HOUR": {1, 1, "a date with a time", ""}, "MINUTE": {1, 1, "a date with a time", ""},
	"DATEADD":       {2, 3, "a date, a number and days|weeks|months|years|hours|minutes", "DATEADD({Start}, 14, \"days\")"},
	"DAYS":          {2, 2, "two dates (the first minus the second)", "DAYS({Due}, TODAY())"},
	"DATETIME_DIFF": {2, 3, "two dates and days|weeks|months|years|hours|minutes", ""}, "DATESTR": {1, 1, "a date", ""},
	"DATE": {3, 3, "year, month, day", "DATE(2026, 12, 31)"}, "IS_BEFORE": {2, 2, "two dates", ""}, "IS_AFTER": {2, 2, "two dates", ""},
	"TEXT": {1, 1, "a value", ""},
}

// Functions is the list for the help sheet: name, what it takes, an example.
func Functions() [][3]string {
	out := make([][3]string, 0, len(funcs))
	for n, f := range funcs {
		if n == "AVG" {
			continue
		}
		out = append(out, [3]string{n, f.args, f.help})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// Eval works out a formula; nil when it cannot (a division by zero, text
// where a number is needed).
func Eval(e Expr, env *Env) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = nil
		}
	}()
	v := eval(e, env)
	if f, ok := v.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
		return nil
	}
	return v
}

type evalErr struct{}

func eval(e Expr, env *Env) any {
	switch t := e.(type) {
	case numLit:
		return t.v
	case strLit:
		return t.v
	case boolLit:
		return t.v
	case fieldRef:
		return env.Get(t.id)
	case unary:
		x := eval(t.x, env)
		switch t.op {
		case "-":
			return -num(x)
		case "!":
			return !truthy(x)
		}
		return num(x)
	case binary:
		switch t.op {
		case "&&":
			return truthy(eval(t.l, env)) && truthy(eval(t.r, env))
		case "||":
			return truthy(eval(t.l, env)) || truthy(eval(t.r, env))
		}
		l, r := eval(t.l, env), eval(t.r, env)
		switch t.op {
		case "&":
			return text(l) + text(r)
		case "+":
			// a date plus days
			if d, ok := asTime(l); ok && isNum(r) {
				return dateOut(l, d.AddDate(0, 0, int(num(r))))
			}
			if isText(l) && !isNum(l) || isText(r) && !isNum(r) {
				return text(l) + text(r)
			}
			return num(l) + num(r)
		case "-":
			if a, ok := asTime(l); ok {
				if b, ok := asTime(r); ok {
					return math.Round(a.Sub(b).Hours()/24*1000) / 1000
				}
				if isNum(r) {
					return dateOut(l, a.AddDate(0, 0, -int(num(r))))
				}
			}
			return num(l) - num(r)
		case "*":
			return num(l) * num(r)
		case "/":
			d := num(r)
			if d == 0 {
				panic(evalErr{})
			}
			return num(l) / d
		case "%":
			d := num(r)
			if d == 0 {
				panic(evalErr{})
			}
			return math.Mod(num(l), d)
		case "^":
			return math.Pow(num(l), num(r))
		case "=", "==":
			return equal(l, r)
		case "!=", "<>":
			return !equal(l, r)
		case "<", ">", "<=", ">=":
			c := compare(l, r)
			switch t.op {
			case "<":
				return c < 0
			case ">":
				return c > 0
			case "<=":
				return c <= 0
			}
			return c >= 0
		}
	case call:
		return callFn(t, env)
	}
	return nil
}

func callFn(c call, env *Env) any {
	arg := func(i int) any { return eval(c.args[i], env) }
	all := func() []any {
		var out []any
		for i := range c.args {
			out = append(out, flat(arg(i))...)
		}
		return out
	}
	nums := func() []float64 {
		var out []float64
		for _, v := range all() {
			if v == nil || v == "" {
				continue
			}
			if n, ok := ToNumber(v); ok {
				out = append(out, n)
			}
		}
		return out
	}
	optInt := func(i, def int) int {
		if i < len(c.args) {
			return int(num(arg(i)))
		}
		return def
	}
	now := env.Now
	if now.IsZero() {
		now = time.Now()
	}
	switch c.fn {
	case "IF":
		if truthy(arg(0)) {
			return arg(1)
		}
		if len(c.args) > 2 {
			return arg(2)
		}
		return nil
	case "AND":
		for _, v := range all() {
			if !truthy(v) {
				return false
			}
		}
		return true
	case "OR":
		for _, v := range all() {
			if truthy(v) {
				return true
			}
		}
		return false
	case "NOT":
		return !truthy(arg(0))
	case "SWITCH":
		v := arg(0)
		i := 1
		for ; i+1 < len(c.args); i += 2 {
			if equal(v, arg(i)) {
				return arg(i + 1)
			}
		}
		if i < len(c.args) {
			return arg(i)
		}
		return nil
	case "SUM":
		s := 0.0
		for _, n := range nums() {
			s += n
		}
		return s
	case "AVERAGE", "AVG":
		ns := nums()
		if len(ns) == 0 {
			return nil
		}
		s := 0.0
		for _, n := range ns {
			s += n
		}
		return s / float64(len(ns))
	case "MIN", "MAX":
		ns := nums()
		if len(ns) == 0 {
			return nil
		}
		m := ns[0]
		for _, n := range ns[1:] {
			if (c.fn == "MIN") == (n < m) {
				m = n
			}
		}
		return m
	case "COUNT":
		n := 0
		for _, v := range all() {
			if v != nil && v != "" && v != false {
				n++
			}
		}
		return float64(n)
	case "ROUND", "ROUNDUP", "ROUNDDOWN":
		p := math.Pow(10, float64(optInt(1, 0)))
		x := math.Round(num(arg(0))*p*1e6) / 1e6 // 18.134999… is 18.135
		switch c.fn {
		case "ROUNDUP":
			x = math.Ceil(x)
		case "ROUNDDOWN":
			x = math.Floor(x)
		default:
			x = math.Round(x)
		}
		return x / p
	case "ABS":
		return math.Abs(num(arg(0)))
	case "SQRT":
		return math.Sqrt(num(arg(0)))
	case "POWER":
		return math.Pow(num(arg(0)), num(arg(1)))
	case "MOD":
		d := num(arg(1))
		if d == 0 {
			panic(evalErr{})
		}
		return math.Mod(num(arg(0)), d)
	case "VALUE":
		n, ok := ToNumber(arg(0))
		if !ok {
			return nil
		}
		return n
	case "CONCAT":
		var b strings.Builder
		for i := range c.args {
			b.WriteString(text(arg(i)))
		}
		return b.String()
	case "TEXT", "DATESTR":
		return text(arg(0))
	case "UPPER":
		return strings.ToUpper(text(arg(0)))
	case "LOWER":
		return strings.ToLower(text(arg(0)))
	case "TRIM":
		return strings.TrimSpace(text(arg(0)))
	case "LEN":
		return float64(len([]rune(text(arg(0)))))
	case "LEFT", "RIGHT":
		r := []rune(text(arg(0)))
		n := max(0, min(int(num(arg(1))), len(r)))
		if c.fn == "LEFT" {
			return string(r[:n])
		}
		return string(r[len(r)-n:])
	case "MID":
		r := []rune(text(arg(0)))
		start := max(int(num(arg(1)))-1, 0)
		if start >= len(r) {
			return ""
		}
		return string(r[start:min(len(r), start+max(0, int(num(arg(2)))))])
	case "FIND":
		hay := strings.ToLower(text(arg(1)))
		i := strings.Index(hay, strings.ToLower(text(arg(0))))
		if i < 0 {
			return 0.0
		}
		return float64(len([]rune(hay[:i])) + 1)
	case "CONTAINS":
		return strings.Contains(strings.ToLower(text(arg(0))), strings.ToLower(text(arg(1))))
	case "SUBSTITUTE":
		return strings.ReplaceAll(text(arg(0)), text(arg(1)), text(arg(2)))
	case "REPT":
		return strings.Repeat(text(arg(0)), max(0, min(int(num(arg(1))), 1000)))
	case "BLANK":
		return nil
	case "ISBLANK":
		return blank(arg(0))
	case "IFBLANK":
		if v := arg(0); !blank(v) {
			return v
		}
		return arg(1)
	case "TODAY":
		return now.Format("2006-01-02")
	case "NOW":
		return now.Format("2006-01-02T15:04")
	case "DATE":
		return time.Date(int(num(arg(0))), time.Month(int(num(arg(1)))), int(num(arg(2))), 0, 0, 0, 0, time.Local).Format("2006-01-02")
	case "YEAR", "MONTH", "DAY", "WEEKDAY", "WEEKNUM", "HOUR", "MINUTE":
		t, ok := asTime(arg(0))
		if !ok {
			return nil
		}
		switch c.fn {
		case "YEAR":
			return float64(t.Year())
		case "MONTH":
			return float64(t.Month())
		case "DAY":
			return float64(t.Day())
		case "WEEKDAY":
			return float64((int(t.Weekday())+6)%7 + 1)
		case "WEEKNUM":
			_, w := t.ISOWeek()
			return float64(w)
		case "HOUR":
			return float64(t.Hour())
		}
		return float64(t.Minute())
	case "DATEADD":
		v := arg(0)
		t, ok := asTime(v)
		if !ok {
			return nil
		}
		n := int(num(arg(1)))
		unit := "days"
		if len(c.args) > 2 {
			unit = strings.ToLower(text(arg(2)))
		}
		switch strings.TrimSuffix(unit, "s") {
		case "week":
			t = t.AddDate(0, 0, 7*n)
		case "month":
			t = t.AddDate(0, n, 0)
		case "year":
			t = t.AddDate(n, 0, 0)
		case "hour":
			return t.Add(time.Duration(n) * time.Hour).Format("2006-01-02T15:04")
		case "minute":
			return t.Add(time.Duration(n) * time.Minute).Format("2006-01-02T15:04")
		default:
			t = t.AddDate(0, 0, n)
		}
		return dateOut(v, t)
	case "DAYS", "DATETIME_DIFF":
		a, ok1 := asTime(arg(0))
		b, ok2 := asTime(arg(1))
		if !ok1 || !ok2 {
			return nil
		}
		unit := "days"
		if len(c.args) > 2 {
			unit = strings.ToLower(text(arg(2)))
		}
		d := a.Sub(b)
		switch strings.TrimSuffix(unit, "s") {
		case "minute":
			return math.Trunc(d.Minutes())
		case "hour":
			return math.Trunc(d.Hours())
		case "week":
			return math.Trunc(d.Hours() / 24 / 7)
		case "month":
			return float64((a.Year()-b.Year())*12 + int(a.Month()) - int(b.Month()))
		case "year":
			return float64(a.Year() - b.Year())
		}
		return math.Round(d.Hours() / 24)
	case "IS_BEFORE", "IS_AFTER":
		a, ok1 := asTime(arg(0))
		b, ok2 := asTime(arg(1))
		if !ok1 || !ok2 {
			return false
		}
		if c.fn == "IS_BEFORE" {
			return a.Before(b)
		}
		return a.After(b)
	}
	return nil
}

func flat(v any) []any {
	switch t := v.(type) {
	case []any:
		var out []any
		for _, x := range t {
			out = append(out, flat(x)...)
		}
		return out
	case []string:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = x
		}
		return out
	}
	return []any{v}
}

func blank(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	case bool:
		return !t
	}
	return false
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != "" && t != "0" && !strings.EqualFold(t, "false")
	case []any:
		return len(t) > 0
	}
	return true
}

func isNum(v any) bool {
	switch t := v.(type) {
	case float64, bool:
		return true
	case string:
		_, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return err == nil
	case []any:
		return len(t) == 1 && isNum(t[0])
	}
	return false
}

func isText(v any) bool {
	_, ok := v.(string)
	return ok
}

func num(v any) float64 {
	if v == nil {
		return 0
	}
	if n, ok := ToNumber(v); ok {
		return n
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return 0
	}
	if a, ok := v.([]any); ok && len(a) == 0 {
		return 0
	}
	panic(evalErr{})
}

func text(v any) string {
	if f, ok := v.(float64); ok {
		return FormatNumber(math.Round(f*1e9)/1e9, -1)
	}
	return ToText(v)
}

func asTime(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		if a, ok := v.([]any); ok && len(a) == 1 {
			return asTime(a[0])
		}
		return time.Time{}, false
	}
	switch {
	case reDate.MatchString(s):
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		return t, err == nil
	case reDateTime.MatchString(s):
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.In(time.Local), true
		}
		t, err := time.ParseInLocation("2006-01-02T15:04", s[:16], time.Local)
		if err != nil {
			t, err = time.ParseInLocation("2006-01-02 15:04", s[:16], time.Local)
		}
		return t, err == nil
	}
	return time.Time{}, false
}

// dateOut keeps a date a date, and a date with a time one with a time.
func dateOut(in any, t time.Time) string {
	if s, ok := in.(string); ok && reDate.MatchString(s) {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02T15:04")
}

func equal(a, b any) bool {
	if a == nil || b == nil {
		return blank(a) && blank(b)
	}
	if isNum(a) && isNum(b) {
		return num(a) == num(b)
	}
	return strings.EqualFold(text(a), text(b))
}

func compare(a, b any) int {
	if ta, ok := asTime(a); ok {
		if tb, ok := asTime(b); ok {
			return ta.Compare(tb)
		}
	}
	if isNum(a) && isNum(b) {
		x, y := num(a), num(b)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
	return strings.Compare(strings.ToLower(text(a)), strings.ToLower(text(b)))
}

// RenameRefs rewrites {Old} as {New} in a formula (a field was renamed).
func RenameRefs(expr, old, nw string) string {
	toks, err := lex(expr)
	if err != nil {
		return strings.ReplaceAll(expr, "{"+old+"}", "{"+nw+"}")
	}
	rs := []rune(expr)
	var b strings.Builder
	last := 0
	for _, t := range toks {
		if t.kind == "field" && strings.EqualFold(t.s, old) {
			end := t.pos
			for end < len(rs) && rs[end] != '}' {
				end++
			}
			b.WriteString(string(rs[last:t.pos]))
			b.WriteString("{" + nw + "}")
			last = end + 1
		}
	}
	b.WriteString(string(rs[last:]))
	return b.String()
}
