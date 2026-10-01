package filemanager

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// restore takes the item of trash step s back out of the trash to where it
// was, and asks what to do when something is there now.
func (r *runner) restore(s step) error {
	if s.to == "" {
		return errNoRestore
	}
	to, skip, err := r.restoreTarget(s)
	if err != nil || skip {
		return err
	}
	if err := r.env.trash.Restore(s.from, s.to, s.at, to); err != nil {
		return err
	}
	r.did(s.to, to)
	return nil
}

// restoreTarget decides where the item of step s goes back to: where it
// was, a free name beside it, or nowhere with skip. A replace moves what
// is there now to the trash first.
func (r *runner) restoreTarget(s step) (to string, skip bool, err error) {
	_, err = os.Lstat(s.from)
	if errors.Is(err, fs.ErrNotExist) {
		return s.from, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("restoring %s: %w", s.from, err)
	}
	ans := answer{}
	if r.all != nil {
		ans = *r.all
	} else {
		if r.env.ask == nil {
			return "", false, fmt.Errorf("%s exists again; move it away to restore the one in the trash", s.from)
		}
		c := clash{src: s.to, dst: s.from, sameKind: true, from: r.env.trash.Describe(s.to)}
		if ans, err = r.env.ask(r.ctx, c); err != nil {
			return "", false, err
		}
		if ans.all {
			r.all = &ans
		}
	}
	switch ans.choice {
	case choiceSkip:
		return "", true, nil
	case choiceKeepBoth:
		to, err := freeName(s.from)
		return to, false, err
	case choiceReplace:
	}
	if _, err := r.env.trash.Trash(s.from); err != nil {
		return "", false, err
	}
	return s.from, false, nil
}
