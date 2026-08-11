package terminal

import (
	"sync"
	"testing"
)

func TestArenaInternsExactEqualsDespiteFingerprintCollision(t *testing.T) {
	arena, ok := New(Config[string]{
		Equal:       func(left, right string) bool { return left == right },
		Fingerprint: func(string) uint64 { return 0 },
	})
	if !ok {
		t.Fatal("arena creation failed")
	}
	first, ok := arena.Admit("left")
	if !ok || first == (ID[string]{}) {
		t.Fatal("first admission failed")
	}
	repeated, ok := arena.Admit("left")
	if !ok || repeated != first {
		t.Fatalf("equal collision admission = %v, want existing %v", repeated, first)
	}
	second, ok := arena.Admit("right")
	if !ok || second == (ID[string]{}) || second == first {
		t.Fatal("unequal collision was merged into one terminal")
	}
	if !arena.Seal() || !arena.Sealed() {
		t.Fatal("terminal arena did not seal")
	}
	if value, valid := arena.Value(first); !valid || value != "left" {
		t.Fatalf("first terminal = %q/%t, want left/true", value, valid)
	}
	if value, valid := arena.Value(second); !valid || value != "right" {
		t.Fatalf("second terminal = %q/%t, want right/true", value, valid)
	}
	if found, valid := arena.Lookup("left"); !valid || found != first {
		t.Fatal("sealed lookup did not recover the canonical identity")
	}
	if _, valid := arena.Lookup("missing"); valid {
		t.Fatal("sealed lookup invented a terminal")
	}
	if _, ok := arena.Admit("after-seal"); ok {
		t.Fatal("sealed terminal universe admitted a new fact value")
	}
}

func TestArenaRejectsUnsealedForeignAndZeroIdentity(t *testing.T) {
	newArena := func() *Arena[int] {
		arena, ok := New(Config[int]{
			Equal:       func(left, right int) bool { return left == right },
			Fingerprint: func(value int) uint64 { return uint64(value) },
		})
		if !ok {
			t.Fatal("arena creation failed")
		}
		return arena
	}
	arena, foreign := newArena(), newArena()
	id, ok := arena.Admit(7)
	if !ok {
		t.Fatal("admission failed")
	}
	if arena.Valid(id) {
		t.Fatal("unsealed terminal identity was accepted by immutable storage")
	}
	if _, ok := arena.Value(id); ok {
		t.Fatal("unsealed terminal value was readable")
	}
	if !arena.Seal() || !foreign.Seal() {
		t.Fatal("seal failed")
	}
	if !arena.Valid(id) {
		t.Fatal("sealed terminal identity was rejected by its owner")
	}
	if foreign.Valid(id) {
		t.Fatal("foreign terminal identity was accepted")
	}
	if _, ok := foreign.Value(id); ok {
		t.Fatal("foreign terminal identity was readable")
	}
	if arena.Valid(ID[int]{}) {
		t.Fatal("zero terminal identity was accepted")
	}
}

func TestWorkPublishesOneImmutableCandidatePage(t *testing.T) {
	config := Config[string]{
		Equal:       func(left, right string) bool { return left == right },
		Fingerprint: func(string) uint64 { return 0 },
	}
	base, ok := New(config)
	if !ok {
		t.Fatal("base creation failed")
	}
	known, ok := base.Admit("known")
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}

	work := base.Begin()
	if work == nil || work.Base() != base {
		t.Fatal("work did not retain its exact sealed base")
	}
	if !work.Valid(known) {
		t.Fatal("work rejected an inherited base identity")
	}
	if value, valid := work.Value(known); !valid || value != "known" {
		t.Fatalf("inherited work value = %q/%t, want known/true", value, valid)
	}
	reused, ok := work.Admit("known")
	if !ok || reused != known {
		t.Fatal("work duplicated an equal base terminal")
	}
	candidate, ok := work.Admit("candidate")
	if !ok || candidate == known || !work.Valid(candidate) {
		t.Fatal("candidate admission failed")
	}
	repeated, ok := work.Admit("candidate")
	if !ok || repeated != candidate {
		t.Fatal("work duplicated an equal candidate terminal")
	}
	if base.Valid(candidate) {
		t.Fatal("candidate identity escaped into the immutable base")
	}
	if _, valid := base.Value(candidate); valid {
		t.Fatal("base resolved an unpublished candidate identity")
	}

	next, ok := work.Seal()
	if !ok || next == base || !next.Sealed() {
		t.Fatal("candidate page did not publish a distinct sealed generation")
	}
	if work.Valid(candidate) {
		t.Fatal("closed work retained candidate authority after publication")
	}
	if !next.Valid(known) || !next.Valid(candidate) {
		t.Fatal("published generation lost an inherited or candidate identity")
	}
	if value, valid := next.Value(candidate); !valid || value != "candidate" {
		t.Fatalf("published candidate value = %q/%t, want candidate/true", value, valid)
	}
	if _, ok := work.Admit("after-seal"); ok {
		t.Fatal("closed work admitted another terminal")
	}
}

func TestArenaGenerationPagesAreBranchLocalAndTransitive(t *testing.T) {
	config := Config[int]{
		Equal:       func(left, right int) bool { return left == right },
		Fingerprint: func(value int) uint64 { return uint64(value) },
	}
	base, ok := New(config)
	if !ok {
		t.Fatal("base creation failed")
	}
	origin, ok := base.Admit(1)
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}

	left := base.Begin()
	right := base.Begin()
	leftID, ok := left.Admit(2)
	if !ok {
		t.Fatal("left candidate admission failed")
	}
	rightID, ok := right.Admit(3)
	if !ok {
		t.Fatal("right candidate admission failed")
	}
	if left.Valid(rightID) || right.Valid(leftID) {
		t.Fatal("sibling work accepted a foreign candidate page")
	}
	leftArena, ok := left.Seal()
	if !ok {
		t.Fatal("left seal failed")
	}
	rightArena, ok := right.Seal()
	if !ok {
		t.Fatal("right seal failed")
	}
	if !base.Valid(leftID) || !base.Valid(rightID) || !leftArena.Valid(rightID) || !rightArena.Valid(leftID) {
		t.Fatal("sealed sibling pages did not join their one semantic terminal owner")
	}

	child := leftArena.Begin()
	childID, ok := child.Admit(4)
	if !ok {
		t.Fatal("child candidate admission failed")
	}
	grandchild, ok := child.Seal()
	if !ok {
		t.Fatal("child seal failed")
	}
	if !grandchild.Valid(origin) || !grandchild.Valid(leftID) || !grandchild.Valid(childID) {
		t.Fatal("descendant generation did not retain its complete ancestor chain")
	}
	if !base.Valid(childID) || !leftArena.Valid(childID) || !rightArena.Valid(childID) {
		t.Fatal("published descendant page did not join its semantic terminal owner")
	}
}

func TestIndependentWorksOnlyShareImmutableTerminalOwner(t *testing.T) {
	base, ok := New(Config[int]{
		Equal:       func(left, right int) bool { return left == right },
		Fingerprint: func(value int) uint64 { return uint64(value) },
	})
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}

	const workers = 32
	ids := make([]ID[int], workers)
	arenas := make([]*Arena[int], workers)
	var group sync.WaitGroup
	group.Add(workers)
	for index := range ids {
		go func(index int) {
			defer group.Done()
			work := base.Begin()
			id, admitted := work.Admit(index + 1)
			published, sealed := work.Seal()
			if !admitted || !sealed {
				return
			}
			ids[index], arenas[index] = id, published
		}(index)
	}
	group.Wait()
	for index, id := range ids {
		if arenas[index] == nil || !arenas[index].Valid(id) || !base.Valid(id) {
			t.Fatalf("independent page %d was not published through the shared owner", index)
		}
	}
}
