package subtype

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestRecordMapInterimTargetForms(t *testing.T) {
	for _, mode := range []Assignability{Gradual, Strict} {
		for _, open := range []bool{false, true} {
			for _, complete := range []bool{false, true} {
				for _, component := range []bool{false, true} {
					for _, empty := range []bool{false, true} {
						t.Run(fmt.Sprintf("%v/open=%v/complete=%v/component=%v/empty=%v", mode, open, complete, component, empty), func(t *testing.T) {
							builder := typ.NewRecord().SetOpen(open).SetComplete(complete)
							if !empty {
								builder.Field("f", typ.String)
							}
							if component {
								builder.MapComponent(typ.String, typ.String)
							}
							for index, target := range []typ.Type{
								typ.NewMap(typ.String, typ.String),
								typ.NewRecord().MapComponent(typ.String, typ.String).Build(),
							} {
								// design-record-to-map.md freezes the v1.6.2 target-form difference.
								want := index == 0 || complete || component
								if got := mode.Assignable(builder.Build(), target); got != want {
									t.Fatalf("map conversion = %v, want %v; target %v", got, want, target)
								}
							}
						})
					}
				}
			}
		}
	}
}

// design-record-to-map.md freezes these historical differences, including the
// empty-record shortcut that precedes inspection of an incompatible component.
func TestRecordMapInterimCompatibility(t *testing.T) {
	cases := []struct {
		name            string
		source          typ.Type
		pure, component bool
	}{
		{"required", typ.NewRecord().Field("f", typ.String).Build(), true, false},
		{"complete", typ.NewRecord().Field("f", typ.String).SetComplete(true).Build(), true, true},
		{"optional", typ.NewRecord().OptField("f", typ.String).SetComplete(true).Build(), true, true},
		{"nilable", typ.NewRecord().Field("f", typ.NewOptional(typ.String)).SetComplete(true).Build(), true, true},
		{"nil-only", typ.NewRecord().Field("f", typ.Nil).SetComplete(true).Build(), true, true},
		{"wrong-value", typ.NewRecord().Field("f", typ.Number).SetComplete(true).Build(), false, false},
		{"wrong-component-key", typ.NewRecord().Field("f", typ.String).MapComponent(typ.Integer, typ.String).Build(), false, false},
		{"wrong-component-value", typ.NewRecord().Field("f", typ.String).MapComponent(typ.String, typ.Number).Build(), false, false},
		{"empty-wrong-component", typ.NewRecord().MapComponent(typ.Integer, typ.Number).Build(), true, false},
	}
	for _, mode := range []Assignability{Gradual, Strict} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%v/%s", mode, tc.name), func(t *testing.T) {
				for i, target := range []typ.Type{typ.NewMap(typ.String, typ.String), typ.NewRecord().MapComponent(typ.String, typ.String).Build()} {
					want := tc.pure
					if i == 1 {
						want = tc.component
					}
					if got := mode.Assignable(tc.source, target); got != want {
						t.Fatalf("%v => %v = %v, want %v", tc.source, target, got, want)
					}
				}
			})
		}
	}
}

func TestRecordMapInterimMutableSlotOpenGuard(t *testing.T) {
	for _, mode := range []Assignability{Gradual, Strict} {
		for _, open := range []bool{false, true} {
			source := typ.NewRecord().Field("slot", typ.NewRecord().Field("f", typ.String).SetOpen(open).Build()).Build()
			target := typ.NewRecord().Field("slot", typ.NewMap(typ.String, typ.String)).Build()
			// design-record-to-map.md retains v1.6.2's nested mutable-slot rule too.
			if got := mode.Assignable(source, target); got != !open {
				t.Fatalf("open=%v: got %v, want %v", open, got, !open)
			}
		}
	}
}
