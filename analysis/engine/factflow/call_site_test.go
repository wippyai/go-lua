package factflow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCalleePathKeyMatchesCallSitePathKey(t *testing.T) {
	callee := path.NewPath(symbol.ID(7), "module").Field("run")
	want := callee.Key()
	key, ok := CalleePathKeyFromPath(callee)
	if !ok || key.PathKey() != want {
		t.Fatalf("CalleePathKeyFromPath = %q/%v, want %q/true", key.PathKey(), ok, want)
	}

	site := NewCallSite(CallSiteConfig{CalleePath: callee}).View()
	if got := site.CalleePathKey(); got != key {
		t.Fatalf("CallSiteView.CalleePathKey() = %q, want %q", got.PathKey(), key.PathKey())
	}

	if got, ok := CalleePathKeyFromPath(path.Path{}); ok || got.Valid() {
		t.Fatalf("empty CalleePathKeyFromPath = %q/%v, want invalid", got.PathKey(), ok)
	}
	if got, ok := CalleePathKeyFromPathKey(""); ok || got.Valid() {
		t.Fatalf("empty CalleePathKeyFromPathKey = %q/%v, want invalid", got.PathKey(), ok)
	}
}

func TestCallSiteOwnsCalleePathKey(t *testing.T) {
	callee := path.NewPath(symbol.ID(9), "module").Field("original")
	site := NewCallSite(CallSiteConfig{CalleePath: callee}).View()
	want := site.CalleePathKey()
	if !want.Valid() {
		t.Fatal("constructed call site did not cache a valid callee key")
	}

	callee.Segments[0].Name = "mutated"
	if got := site.CalleePathKey(); got != want {
		t.Fatalf("CallSiteView.CalleePathKey changed after caller mutation: %q != %q", got.PathKey(), want.PathKey())
	}
	if got := site.CalleePath().Key(); got != want.PathKey() {
		t.Fatalf("owned callee path key = %q, want cached key %q", got, want.PathKey())
	}
}

func TestCallSiteCarriesMemberAccessEvidence(t *testing.T) {
	direct := NewCallSite(CallSiteConfig{
		CalleePath: path.Path{Root: "math.max"},
	}).View()
	if direct.CalleeMemberAccess() {
		t.Fatal("punctuated direct callee root reported member access")
	}

	member := NewCallSite(CallSiteConfig{
		CalleePath:         path.Path{Root: "api"}.Field("make"),
		CalleeMemberAccess: true,
	}).View()
	if !member.CalleeMemberAccess() {
		t.Fatal("member call site dropped member-access evidence")
	}
}

func TestCallSiteMemberAccessPath(t *testing.T) {
	receiver := path.NewPath(symbol.ID(13), "svc")
	colon := NewCallSite(CallSiteConfig{
		CalleePath:         receiver.Field("run"),
		CalleeMemberAccess: true,
		ReceiverPath:       receiver,
		HasReceiverPath:    true,
		MethodName:         "run",
	}).View()
	gotReceiver, gotMember, ok := colon.CalleeMemberAccessPath()
	if !ok || !gotReceiver.Equal(receiver) || gotMember.Kind != segment.SegmentField || gotMember.Name != "run" {
		t.Fatalf("colon member access = %v/%#v/%v, want %v/.run/true", gotReceiver, gotMember, ok, receiver)
	}

	dottedPath := path.NewPath(symbol.ID(17), "api").Field("make")
	dotted := NewCallSite(CallSiteConfig{
		CalleePath:         dottedPath,
		CalleeMemberAccess: true,
	}).View()
	gotReceiver, gotMember, ok = dotted.CalleeMemberAccessPath()
	if !ok || !gotReceiver.Equal(path.NewPath(symbol.ID(17), "api")) || gotMember.Kind != segment.SegmentField || gotMember.Name != "make" {
		t.Fatalf("dotted member access = %v/%#v/%v", gotReceiver, gotMember, ok)
	}

	dottedPath.Segments[0].Name = "mutated"
	againReceiver, againMember, ok := dotted.CalleeMemberAccessPath()
	if !ok || againReceiver.String() == "api.mutated" || againMember.Name != "make" {
		t.Fatalf("member access path exposed caller mutation: %v/%#v/%v", againReceiver, againMember, ok)
	}
}

func TestCallResultTargetOwnsTargetPathKey(t *testing.T) {
	targetPath := path.NewPath(symbol.ID(11), "target").Field("original")
	target := NewCallResultTarget(CallResultTargetLocalAssignment, 0, 0, symbol.ID(11), targetPath)
	view := CallResultTargetView{target: target}
	want := view.TargetPathKey()
	if want == "" {
		t.Fatal("constructed call result target did not cache a target key")
	}

	targetPath.Segments[0].Name = "mutated"
	if got := view.TargetPathKey(); got != want {
		t.Fatalf("CallResultTargetView.TargetPathKey changed after caller mutation: %q != %q", got, want)
	}
	if got := view.TargetPath().Key(); got != want {
		t.Fatalf("owned target path key = %q, want cached key %q", got, want)
	}
	if got := view.TargetPathRef().Key(); got != want {
		t.Fatalf("target path ref key = %q, want cached key %q", got, want)
	}
}
