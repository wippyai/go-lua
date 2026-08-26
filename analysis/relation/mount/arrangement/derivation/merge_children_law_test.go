package derivation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestMergeFrameSealsEveryAuthoredChildVector(t *testing.T) {
	fixture := newShapeFixture(t)
	plan, ok := Build(fixture.root, fixture.expression, fixture.bindings, fixture.inputs, []signature.Signature{fixture.signature})
	if !ok || !plan.Available() {
		t.Fatal("derivation")
	}
	for index := 0; index < plan.Len(); index++ {
		path, pathOK := plan.PathAt(index)
		if !pathOK {
			t.Fatal("path")
		}
		for frameIndex := 0; frameIndex < path.FrameCount(); frameIndex++ {
			frame, frameOK := path.FrameAt(frameIndex)
			if !frameOK || frame.Kind() != algebra.KindMerge {
				continue
			}
			if frame.ChildCount() != frame.SiblingCount()+1 || frame.ChildCount() != 2 {
				t.Fatalf("path %d merge children=%d siblings=%d", index, frame.ChildCount(), frame.SiblingCount())
			}
			for childIndex := 0; childIndex < frame.ChildCount(); childIndex++ {
				child, childOK := frame.ChildAt(childIndex)
				if !childOK || !child.Available() || child.Access().Key().Available() || child.Ordinal() != uint32(childIndex) || !child.Node().Available() {
					t.Fatalf("path %d merge child %d unavailable/unkeyed contract", index, childIndex)
				}
			}
			active := 0
			for childIndex := 0; childIndex < frame.ChildCount(); childIndex++ {
				child, _ := frame.ChildAt(childIndex)
				if child.Ordinal() == frame.Ordinal() {
					active++
				}
			}
			if active != 1 {
				t.Fatalf("path %d merge active child count=%d", index, active)
			}
			first, firstOK := frame.ChildAt(0)
			second, secondOK := frame.ChildAt(1)
			if !firstOK || !secondOK || first.Kind() != algebra.KindApply || second.Kind() != algebra.KindColumnProject || first.Node() == second.Node() {
				t.Fatalf("path %d merge Apply/carry witnesses lost node or kind distinction", index)
			}
		}
	}
}

func TestMergeChildWitnessTamperingInvalidatesPath(t *testing.T) {
	fixture := newShapeFixture(t)
	plan, ok := Build(fixture.root, fixture.expression, fixture.bindings, fixture.inputs, []signature.Signature{fixture.signature})
	if !ok || !plan.Available() {
		t.Fatal("derivation")
	}
	path, ok := plan.PathAt(0)
	if !ok {
		t.Fatal("path")
	}
	mergeIndex := -1
	for index, frame := range path.frames {
		if frame.Kind() == algebra.KindMerge {
			mergeIndex = index
			break
		}
	}
	if mergeIndex < 0 {
		t.Fatal("merge frame")
	}
	mutated := func(change func(*ChildWitness)) Path {
		copyOf := path
		copyOf.frames = append([]Frame(nil), path.frames...)
		frame := copyOf.frames[mergeIndex]
		frame.children = append([]ChildWitness(nil), frame.children...)
		child := frame.children[0]
		change(&child)
		frame.children[0] = child
		copyOf.frames[mergeIndex] = frame
		return copyOf
	}
	for name, change := range map[string]func(*ChildWitness){
		"kind":     func(child *ChildWitness) { child.kind = algebra.KindInput },
		"node":     func(child *ChildWitness) { child.node = identity.ContentID{250} },
		"physical": func(child *ChildWitness) { child.value.physical = identity.ContentID{251} },
	} {
		if mutated(change).Available() {
			t.Fatalf("tampered merge child %s remained available", name)
		}
	}
	tamperedDigest := path
	tamperedDigest.digest = identity.ContentID{252}
	if tamperedDigest.Available() {
		t.Fatal("tampered path digest remained available")
	}
}
