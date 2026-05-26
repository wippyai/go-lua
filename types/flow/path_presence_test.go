package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestPathPresenceJoin_MissingBranchMakesValueOptional(t *testing.T) {
	presence := joinPathPresence(pathPresencePresent, pathPresenceAbsent)
	got := projectPathPresence(typ.String, presence)
	want := typ.NewOptional(typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("projectPathPresence(present join absent) = %v, want %v", got, want)
	}
}

func TestPathPresenceProject_PresentStripsNil(t *testing.T) {
	got := projectPathPresence(typ.NewOptional(typ.String), pathPresencePresent)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("projectPathPresence(present optional string) = %v, want string", got)
	}
}

func TestPathPresenceProject_AbsentIsNil(t *testing.T) {
	got := projectPathPresence(typ.String, pathPresenceAbsent)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("projectPathPresence(absent string) = %v, want nil", got)
	}
}
