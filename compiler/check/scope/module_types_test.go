package scope

import (
	"testing"

	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWithModuleTypesBindsUniqueNames(t *testing.T) {
	elem := typ.NewTypeParam("T", nil)
	channelGeneric := typ.NewGeneric("channel.Channel", []*typ.TypeParam{elem}, typ.NewInterface("channel.Channel", nil))
	channel := io.NewManifest("channel")
	channel.DefineType("Channel", channelGeneric)

	timeType := typ.NewInterface("time.Time", nil)
	clock := io.NewManifest("time")
	clock.DefineType("Time", timeType)
	// A module may re-export another module's type under the same name.
	clock.DefineType("Channel", channelGeneric)

	s := NewWithBuiltins().WithModuleTypes([]*io.Manifest{clock, channel})

	if got, ok := s.LookupType("Channel"); !ok || got != channelGeneric {
		t.Fatalf("Channel = %v, %v; want the channel generic", got, ok)
	}
	if got, ok := s.LookupType("Time"); !ok || got != timeType {
		t.Fatalf("Time = %v, %v; want time.Time", got, ok)
	}
}

func TestWithModuleTypesLeavesConflictingNamesUnbound(t *testing.T) {
	sqlResult := typ.NewRecord().Field("rows", typ.Integer).Build()
	httpResult := typ.NewRecord().Field("status", typ.Integer).Build()
	sql := io.NewManifest("sql")
	sql.DefineType("Result", sqlResult)
	http := io.NewManifest("http")
	http.DefineType("Result", httpResult)

	for _, order := range [][]*io.Manifest{{sql, http}, {http, sql}} {
		s := NewWithBuiltins().WithModuleTypes(order)
		if got, ok := s.LookupType("Result"); ok {
			t.Fatalf("Result must stay unbound when modules disagree, got %v", got)
		}
	}
}

func TestWithModuleTypesKeepsBuiltinNames(t *testing.T) {
	m := io.NewManifest("custom")
	m.DefineType("string", typ.NewRecord().Build())

	s := NewWithBuiltins().WithModuleTypes([]*io.Manifest{m})
	if got, _ := s.LookupType("string"); got != typ.String {
		t.Fatalf("builtin string must not be shadowed, got %v", got)
	}
}
