package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// fn is an expression of x, ready to evaluate.
type fn func(x float64) float64

// parse reads an expression: numbers, x, π and e, + − × ÷ ^ and %,
// brackets, and sin cos tan √ ln log abs. A number or a closing bracket
// followed by x, a bracket or a function multiplies, so 2x and 3(x+1)
// read as they are written.
func parse(src string) (fn, error) {
	p := &parser{toks: tokens(src)}
	if len(p.toks) == 0 {
		return nil, errors.New("nothing to work out")
	}
	f, err := p.sum()
	if err != nil {
		return nil, err
	}
	if p.at < len(p.toks) {
		return nil, fmt.Errorf("%q is out of place", p.toks[p.at])
	}
	return f, nil
}

// usesX reports whether an expression names x, which makes it a curve
// to draw rather than a sum to work out.
func usesX(src string) bool {
	for _, t := range tokens(src) {
		if t == "x" {
			return true
		}
	}
	return false
}

// evaluate works an expression out, with no x.
func evaluate(src string) (float64, error) {
	f, err := parse(src)
	if err != nil {
		return 0, err
	}
	v := f(0)
	if math.IsNaN(v) {
		return 0, errors.New("that has no answer")
	}
	if math.IsInf(v, 0) {
		return 0, errors.New("that is too big")
	}
	return v, nil
}

// format writes a number as a calculator shows it: up to twelve
// significant digits, without trailing zeros.
func format(v float64) string {
	if v == 0 {
		return "0"
	}
	if a := math.Abs(v); a >= 1e12 || a < 1e-9 {
		s := strconv.FormatFloat(v, 'e', 8, 64)
		mant, exp, _ := strings.Cut(s, "e")
		if strings.Contains(mant, ".") {
			mant = strings.TrimRight(strings.TrimRight(mant, "0"), ".")
		}
		return mant + "e" + strings.TrimLeft(exp, "+")
	}
	s := strconv.FormatFloat(v, 'g', 12, 64)
	if strings.Contains(s, ".") && !strings.Contains(s, "e") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return strings.Replace(s, "-", "−", 1)
}

// functions are the names that take a bracket after them.
var functions = map[string]func(float64) float64{
	"sin": math.Sin, "cos": math.Cos, "tan": math.Tan,
	"√": math.Sqrt, "sqrt": math.Sqrt,
	"ln": math.Log, "log": math.Log10, "abs": math.Abs,
}

// tokens splits an expression into numbers, names and signs. The signs
// a keypad writes, × ÷ −, read as * / -.
func tokens(src string) []string {
	var out []string
	rs := []rune(src)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case unicode.IsDigit(r) || r == '.':
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			out = append(out, string(rs[i:j]))
			i = j
		case unicode.IsLetter(r) && r != 'π':
			j := i
			for j < len(rs) && unicode.IsLetter(rs[j]) && rs[j] != 'π' {
				j++
			}
			// A run of letters is functions and letters each on its
			// own, the longest function at each place first: xx is x
			// times x, and xsin(x), as the keypad types it, is x times
			// sin(x).
			out = append(out, words(rs[i:j])...)
			i = j
		default:
			s := string(r)
			switch r {
			case '×', '·':
				s = "*"
			case '÷':
				s = "/"
			case '−':
				s = "-"
			}
			out = append(out, s)
			i++
		}
	}
	return out
}

// words splits a run of letters into the functions it names, longest
// first, and the letters between them.
func words(rs []rune) []string {
	var out []string
	for k := 0; k < len(rs); {
		best := ""
		for name := range functions {
			if len(name) > len(best) && strings.HasPrefix(string(rs[k:]), name) {
				best = name
			}
		}
		if best == "" {
			best = string(rs[k])
		}
		out = append(out, best)
		k += len([]rune(best))
	}
	return out
}

type parser struct {
	toks []string
	at   int
}

func (p *parser) peek() string {
	if p.at < len(p.toks) {
		return p.toks[p.at]
	}
	return ""
}

func (p *parser) next() string {
	t := p.peek()
	p.at++
	return t
}

// sum is terms joined by + and -.
func (p *parser) sum() (fn, error) {
	f, err := p.product()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek() {
		case "+":
			p.next()
			g, err := p.product()
			if err != nil {
				return nil, err
			}
			a := f
			f = func(x float64) float64 { return a(x) + g(x) }
		case "-":
			p.next()
			g, err := p.product()
			if err != nil {
				return nil, err
			}
			a := f
			f = func(x float64) float64 { return a(x) - g(x) }
		default:
			return f, nil
		}
	}
}

// startsFactor reports whether a token begins a factor, for a product
// written with no sign between.
func startsFactor(t string) bool {
	if t == "(" || t == "x" || t == "π" || t == "e" || t == "√" {
		return true
	}
	if _, ok := functions[t]; ok {
		return true
	}
	return t != "" && (unicode.IsDigit(rune(t[0])) || t[0] == '.')
}

// product is powers joined by *, / and %, or by nothing.
func (p *parser) product() (fn, error) {
	f, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		op := p.peek()
		switch {
		case op == "*" || op == "/" || op == "%":
			p.next()
		case startsFactor(op):
			op = "*"
		default:
			return f, nil
		}
		g, err := p.unary()
		if err != nil {
			return nil, err
		}
		a := f
		switch op {
		case "*":
			f = func(x float64) float64 { return a(x) * g(x) }
		case "/":
			f = func(x float64) float64 { return a(x) / g(x) }
		default:
			f = func(x float64) float64 { return math.Mod(a(x), g(x)) }
		}
	}
}

// unary is a power with signs in front.
func (p *parser) unary() (fn, error) {
	switch p.peek() {
	case "-":
		p.next()
		f, err := p.unary()
		if err != nil {
			return nil, err
		}
		return func(x float64) float64 { return -f(x) }, nil
	case "+":
		p.next()
		return p.unary()
	}
	return p.power()
}

// power is an atom raised, right to left: 2^3^2 is 2^9.
func (p *parser) power() (fn, error) {
	base, err := p.atom()
	if err != nil {
		return nil, err
	}
	if p.peek() != "^" {
		return base, nil
	}
	p.next()
	exp, err := p.unary()
	if err != nil {
		return nil, err
	}
	return func(x float64) float64 { return math.Pow(base(x), exp(x)) }, nil
}

// atom is a number, a name, a function of a bracket, or a bracket.
func (p *parser) atom() (fn, error) {
	t := p.next()
	switch t {
	case "":
		return nil, errors.New("it ends too soon")
	case "x":
		return func(x float64) float64 { return x }, nil
	case "π":
		return func(float64) float64 { return math.Pi }, nil
	case "e":
		return func(float64) float64 { return math.E }, nil
	case "(":
		f, err := p.sum()
		if err != nil {
			return nil, err
		}
		// A bracket left open closes at the end, as on a calculator.
		if p.peek() == ")" {
			p.next()
		}
		return f, nil
	}
	if g, ok := functions[t]; ok {
		arg, err := p.power()
		if err != nil {
			return nil, err
		}
		return func(x float64) float64 { return g(arg(x)) }, nil
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return nil, fmt.Errorf("%q is no number", t)
	}
	return func(float64) float64 { return v }, nil
}
