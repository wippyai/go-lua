package subtype

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestRecordMapEvidenceUsesOneRule(t *testing.T) {
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
							for _, target := range []typ.Type{
								typ.NewMap(typ.String, typ.String),
								typ.NewRecord().MapComponent(typ.String, typ.String).Build(),
							} {
								if got := mode.Assignable(builder.Build(), target); got != (complete || component) {
									t.Fatalf("map conversion = %v, want %v; target %v", got, complete || component, target)
								}
							}
						})
					}
				}
			}
		}
	}
}
