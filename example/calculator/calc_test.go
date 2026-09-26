package main

import (
	"math"
	"testing"
)

func TestSumsWorkOutAsWritten(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
	}{
		{"1+2×3", 7},
		{"(1+2)×3", 9},
		{"2^3^2", 512},
		{"−2^2", -4},
		{"10÷4", 2.5},
		{"2π", 2 * math.Pi},
		{"3(1+1)", 6},
		{"√16+ln(e)", 5},
		{"sin(0)+cos(0)", 1},
		{"(2+3", 5},
		{"7%3", 1},
	} {
		got, err := evaluate(c.in)
		if err != nil || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
}

func TestACurveOfXEvaluatesAtEachX(t *testing.T) {
	f, err := parse("2x^2−sin(x)")
	if err != nil {
		t.Fatal(err)
	}
	if got := f(2); math.Abs(got-(8-math.Sin(2))) > 1e-9 {
		t.Fatalf("at 2 it is %v", got)
	}
	if !usesX("2x+1") || usesX("2+1") || usesX("sin(2)") {
		t.Fatal("usesX is wrong about which names x")
	}
}

func TestWrongSumsSaySo(t *testing.T) {
	for _, in := range []string{"", "1+", "1/0", "√(−1)", "2**3"} {
		if _, err := evaluate(in); err == nil {
			t.Errorf("%q worked out", in)
		}
	}
}

func TestNumbersShowAsACalculatorShowsThem(t *testing.T) {
	for _, c := range []struct {
		v    float64
		want string
	}{
		{0, "0"}, {2.5, "2.5"}, {-3, "−3"}, {1.0 / 3, "0.333333333333"},
		{1e15, "1e15"}, {123456, "123456"},
	} {
		if got := format(c.v); got != c.want {
			t.Errorf("%v shows as %q, want %q", c.v, got, c.want)
		}
	}
}
