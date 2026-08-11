package binding

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestSealRejectsExpiredSourcePreimageIndependently(t *testing.T) {
	bodies, bodyFinish := liveLifecycleBodyResult(t)
	defer bodyFinish()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}})
	defer func() { _ = flowFinalizer.Abort() }()
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	if err := sourceFinalizer.Abort(); err != nil {
		t.Fatalf("source Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("expired Source Preimage was accepted")
	}
}

func TestSealRejectsExpiredAuthoredViewIndependently(t *testing.T) {
	bodies, bodyFinish := liveLifecycleBodyResult(t)
	defer bodyFinish()
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	defer func() { _ = sourceFinalizer.Abort() }()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}})
	if err := flowFinalizer.Abort(); err != nil {
		t.Fatalf("authored Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("expired authored View was accepted")
	}
}

func TestSealExpiredOwnersOnZeroFamilyEmptyModel(t *testing.T) {
	bodies, bodyFinish := liveLifecycleBodyResult(t)
	defer bodyFinish()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{})
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	if err := sourceFinalizer.Abort(); err != nil {
		t.Fatalf("source Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("zero-family model with expired Source was accepted")
	}
	if err := flowFinalizer.Abort(); err != nil {
		t.Fatalf("authored Abort: %v", err)
	}

	preimage, sourceFinalizer = liveLifecycleSource(t, 1)
	view, flowFinalizer = liveLifecycleAuthored(t, authored.Input{})
	if err := flowFinalizer.Abort(); err != nil {
		t.Fatalf("authored Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("zero-family model with expired authored View was accepted")
	}
	if err := sourceFinalizer.Abort(); err != nil {
		t.Fatalf("source Abort: %v", err)
	}
}

func liveLifecycleBodyResult(t *testing.T) (*body.Result, func()) {
	t.Helper()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}})
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	result, err := body.Seal(preimage, view, staticFinalizer.View(), keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		_ = flowFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		_ = staticFinalizer.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	return result, func() {
		_ = flowFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		_ = staticFinalizer.Abort()
	}
}

func liveLifecycleAuthored(t *testing.T, input authored.Input) (authored.View, authored.Finalizer) {
	t.Helper()
	draft, err := authored.Build(input)
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	finish, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	return finish.View(), finish
}

func liveLifecycleSource(t *testing.T, bodyCount int) (source.Preimage, source.Finalizer) {
	t.Helper()
	name := "binding-lifecycle.lua"
	input := source.Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := 0
		if family == keyspace.FamilyBody {
			count = bodyCount
		}
		spans := make([]source.Span, count)
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, bodyCount)
	for index := range input.Bodies {
		input.Bodies[index].Body = keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
	}
	draft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	finish, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	return finish.Preimage(), finish
}
