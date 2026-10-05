package install

import (
	"strconv"
	"strings"
)

// A version is "v1.2.3" or "1.2.3", with perhaps a pre-release after a
// dash, as "v1.2.3-beta.2", and perhaps build data after a plus, which
// counts for nothing. Anything else, as "dev" or a commit, is no
// release.

// semver is a version taken apart.
type semver struct {
	nums [3]int
	pre  []string
}

// parse takes v apart, and says whether it is a version at all.
func parse(v string) (semver, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "+")
	core, pre, hasPre := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	var s semver
	if len(parts) != 3 {
		return s, false
	}
	for i, p := range parts {
		n, ok := number(p)
		if !ok {
			return s, false
		}
		s.nums[i] = n
	}
	if hasPre {
		if pre == "" {
			return s, false
		}
		s.pre = strings.Split(pre, ".")
		for _, id := range s.pre {
			if id == "" {
				return s, false
			}
		}
	}
	return s, true
}

// number reads a part of a version: digits, with no leading zero.
func number(p string) (int, bool) {
	n, err := strconv.Atoi(p)
	if err != nil || n < 0 || strconv.Itoa(n) != p {
		return 0, false
	}
	return n, true
}

// IsRelease reports whether v names a release, as a build from a working
// tree, calling itself "dev" or a commit, does not.
func IsRelease(v string) bool {
	_, ok := parse(v)
	return ok
}

// compare orders two versions: -1 when a is the earlier, 1 when it is
// the later, and 0 when they are the same, or when either is no
// version and there is no order to give.
func compare(a, b string) int {
	x, ok := parse(a)
	if !ok {
		return 0
	}
	y, ok := parse(b)
	if !ok {
		return 0
	}
	for i := range x.nums {
		switch {
		case x.nums[i] < y.nums[i]:
			return -1
		case x.nums[i] > y.nums[i]:
			return 1
		}
	}
	// A pre-release comes before its release.
	switch {
	case len(x.pre) == 0 && len(y.pre) == 0:
		return 0
	case len(x.pre) == 0:
		return 1
	case len(y.pre) == 0:
		return -1
	}
	for i := 0; i < len(x.pre) && i < len(y.pre); i++ {
		if c := comparePre(x.pre[i], y.pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(x.pre) < len(y.pre):
		return -1
	case len(x.pre) > len(y.pre):
		return 1
	}
	return 0
}

// comparePre orders two parts of a pre-release: numbers by value and
// before words, words as text.
func comparePre(a, b string) int {
	m, aNum := number(a)
	n, bNum := number(b)
	switch {
	case aNum && bNum:
		switch {
		case m < n:
			return -1
		case m > n:
			return 1
		}
		return 0
	case aNum:
		return -1
	case bNum:
		return 1
	}
	return strings.Compare(a, b)
}

// Newer reports whether version a is later than version b.
func Newer(a, b string) bool { return compare(a, b) > 0 }
