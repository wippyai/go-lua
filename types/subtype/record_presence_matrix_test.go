package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestRecordPresenceAndValueDomains(t *testing.T) {
	forms := []struct {
		name     string
		value    typ.Type
		optional bool
		concrete bool
		required bool
	}{
		{"f: T", typ.String, false, true, true},
		{"f?: T", typ.String, true, true, false},
		{"f: T?", typ.NewOptional(typ.String), false, true, false},
		{"f?: T?", typ.NewOptional(typ.String), true, true, false},
		{"f: any", typ.Any, false, false, false},
		{"f?: any", typ.Any, true, false, false},
		{"f: unknown", typ.Unknown, false, false, false},
	}
	for _, mode := range []Assignability{Gradual, Strict} {
		modeName := "gradual"
		if mode == Strict {
			modeName = "strict"
		}
		for _, source := range forms {
			for _, target := range forms {
				t.Run(modeName+"/"+source.name+" -> "+target.name, func(t *testing.T) {
					sub := typ.NewRecord().AddField(typ.Field{Name: "f", Type: source.value, Optional: source.optional}).Build()
					super := typ.NewRecord().AddField(typ.Field{Name: "f", Type: target.value, Optional: target.optional}).Build()
					want := true
					if target.required {
						want = source.required || (mode == Gradual && !source.concrete && !source.optional)
					}
					if target.concrete && !source.concrete && mode == Strict {
						want = false
					}
					if got := mode.Assignable(sub, super); got != want {
						t.Fatalf("assignable = %v, want %v", got, want)
					}
				})
			}
		}
	}
}

func TestRecordAbsenceAndWrongPresentValues(t *testing.T) {
	for _, mode := range []Assignability{Gradual, Strict} {
		for _, target := range []typ.Type{typ.Any, typ.Unknown, typ.Nil, typ.NewOptional(typ.String), typ.NewUnion(typ.String, typ.Boolean, typ.Nil)} {
			super := typ.NewRecord().Field("f", target).Build()
			for _, sub := range []*typ.Record{typ.NewRecord().Build(), typ.NewRecord().Field("f", typ.Nil).Build()} {
				if !mode.Assignable(sub, super) {
					t.Errorf("mode=%v: absent field must fit %v", mode, super)
				}
			}
		}
		for _, target := range []typ.Type{typ.String, typ.NewOptional(typ.String)} {
			super := typ.NewRecord().OptField("f", target).Build()
			if mode.Assignable(typ.NewRecord().OptField("f", typ.Integer).Build(), super) {
				t.Errorf("mode=%v: integer present value must not fit %v", mode, super)
			}
		}
		if mode.Assignable(typ.NewRecord().Field("f", typ.Nil).Build(), typ.NewRecord().Field("f", typ.String).Build()) {
			t.Errorf("mode=%v: nil cannot supply a required string", mode)
		}
	}
}
