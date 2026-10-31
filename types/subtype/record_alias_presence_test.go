package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestRecordPresentAliasMatchesLocalReference(t *testing.T) {
	value := typ.NewAlias("Options", typ.NewRecord().Field("size", typ.Integer).Build())
	for _, mode := range []Assignability{Gradual, Strict} {
		target := typ.NewRecord().Field("options", typ.NewRef("", "Options")).Build()
		if !mode.Assignable(typ.NewRecord().Field("options", value).Build(), target) {
			t.Errorf("mode=%v: non-nil alias identity lost in field comparison", mode)
		}
		for _, wrong := range []typ.Type{
			typ.NewOptional(value),
			typ.NewAlias("OtherOptions", value.Target),
		} {
			if mode.Assignable(typ.NewRecord().Field("options", wrong).Build(), target) {
				t.Errorf("mode=%v: accepted absent or differently named reference %v", mode, wrong)
			}
		}
	}
}
