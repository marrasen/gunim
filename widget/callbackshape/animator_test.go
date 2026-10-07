package callbackshape_test

import (
	"reflect"
	"testing"

	"github.com/marrasen/gunim"
)

// A widget that embeds an animated one is animated too: a field of its own named Step would hide the embedded Step,
// and the engine would step nothing, as a number field's once did.
func TestAWidgetEmbeddingAnAnimatorAnimates(t *testing.T) {
	animator := reflect.TypeFor[gunim.Animator]()
	for _, s := range structs {
		ty := reflect.TypeOf(s)
		for _, f := range reflect.VisibleFields(ty.Elem()) {
			if !f.Anonymous || len(f.Index) != 1 {
				continue
			}
			ft := f.Type
			if ft.Kind() != reflect.Pointer {
				ft = reflect.PointerTo(ft)
			}
			if ft.Implements(animator) && !ty.Implements(animator) {
				t.Errorf("%s embeds %s, which animates, and is no gunim.Animator itself", ty.Elem(), f.Type)
			}
		}
	}
}
