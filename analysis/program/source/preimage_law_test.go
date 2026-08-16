package source

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPreimageBindsEverySubviewToOneOwner(t *testing.T) {
	firstInput := preimageOwnerInput("first.lua", false, "first", ControlFaultUndefinedGoto, true)
	secondInput := preimageOwnerInput("second.lua", true, "second", ControlFaultBreakOutsideLoop, false)
	firstDraft, err := Build(firstInput)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	secondDraft, err := Build(secondInput)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	firstFinalizer, err := firstDraft.Finalizer()
	if err != nil {
		t.Fatalf("first Finalizer: %v", err)
	}
	secondFinalizer, err := secondDraft.Finalizer()
	if err != nil {
		t.Fatalf("second Finalizer: %v", err)
	}
	first := firstFinalizer.Preimage()
	second := secondFinalizer.Preimage()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	cellOne := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cellTwo := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)

	if got := first.Identity().Name(); got != firstInput.Name {
		t.Fatalf("first Identity.Name() = %q, want %q", got, firstInput.Name)
	}
	if got := second.Identity().Name(); got != secondInput.Name {
		t.Fatalf("second Identity.Name() = %q, want %q", got, secondInput.Name)
	}
	if got, ok := first.Order().BodyAt(body, 0); !ok || got != bind {
		t.Fatalf("first Order.BodyAt() = %v/%v, want Bind", got, ok)
	}
	if got, ok := second.Order().BodyAt(body, 0); !ok || got != fault {
		t.Fatalf("second Order.BodyAt() = %v/%v, want ControlFault", got, ok)
	}
	if got, ok := first.Binds().At(bind, 0); !ok || got != cellOne {
		t.Fatalf("first Binds.At() = %v/%v, want Cell1", got, ok)
	}
	if got, ok := second.Binds().At(bind, 0); !ok || got != cellTwo {
		t.Fatalf("second Binds.At() = %v/%v, want Cell2", got, ok)
	}
	if got, ok := first.Formals().At(function, 0); !ok || got != cellTwo {
		t.Fatalf("first Formals.At() = %v/%v, want Cell2", got, ok)
	}
	if got, ok := second.Formals().At(function, 0); !ok || got != cellOne {
		t.Fatalf("second Formals.At() = %v/%v, want Cell1", got, ok)
	}
	if _, got, _, ok := first.Keys().Name(key); !ok || got != "first" {
		t.Fatalf("first Keys.Name() = %q/%v, want first", got, ok)
	}
	if _, got, _, ok := second.Keys().Name(key); !ok || got != "second" {
		t.Fatalf("second Keys.Name() = %q/%v, want second", got, ok)
	}
	if got, ok := first.Faults().At(fault); !ok || got.Kind != ControlFaultUndefinedGoto {
		t.Fatalf("first Faults.At() = %#v/%v, want UndefinedGoto", got, ok)
	}
	if got, ok := second.Faults().At(fault); !ok || got.Kind != ControlFaultBreakOutsideLoop {
		t.Fatalf("second Faults.At() = %#v/%v, want BreakOutsideLoop", got, ok)
	}
	if _, _, got, ok := first.Literals().Bools().At(0); !ok || !got {
		t.Fatalf("first Literals.Bools.At() = %v/%v, want true", got, ok)
	}
	if _, _, got, ok := second.Literals().Bools().At(0); !ok || got {
		t.Fatalf("second Literals.Bools.At() = %v/%v, want false", got, ok)
	}
}

func preimageOwnerInput(name string, swapOrder bool, keyName string, faultKind ControlFaultKind, boolValue bool) Input {
	input, _ := sourceFixture(1)
	input.Name = name
	for family := range input.Families {
		for span := range input.Families[family].Spans {
			input.Families[family].Spans[span].File = name
		}
	}
	input.Binds[0].Cells = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1)}
	input.Functions[0].Formals = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 2)}
	if swapOrder {
		input.Binds[0].Cells[0], input.Functions[0].Formals[0] = input.Functions[0].Formals[0], input.Binds[0].Cells[0]
	}
	input.Bool[0].Value = boolValue
	input.Families[int(keyspace.FamilyKey)-1].Spans = []Span{{File: name, StartLine: 20, StartCol: 1, EndLine: 20, EndCol: 1}}
	input.ExactAtoms = []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: keyName}}
	input.Keys = []KeyInput{NameKey(keyspace.MakeTerm(keyspace.FamilyBody, 1), keyName)}
	input.Families[int(keyspace.FamilyControlFault)-1].Spans = []Span{{File: name, StartLine: 21, StartCol: 1, EndLine: 21, EndCol: 1}}
	input.Faults = []ControlFault{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Kind: faultKind}}
	input.Bodies[0].Terms = append(input.Bodies[0].Terms, keyspace.MakeTerm(keyspace.FamilyControlFault, 1))
	if swapOrder {
		input.Bodies[0].Terms[0], input.Bodies[0].Terms[1] = input.Bodies[0].Terms[1], input.Bodies[0].Terms[0]
	}
	return input
}

func TestPreimageCapturedSubviewsExpireAfterCommit(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	preimage := finalizer.Preimage()
	identity := preimage.Identity()
	order := preimage.Order()
	binds := preimage.Binds()
	formals := preimage.Formals()
	keys := preimage.Keys()
	faults := preimage.Faults()
	literals := preimage.Literals()
	nils := literals.Nils()
	if _, err := finalizer.Commit(ownedIndex(draft, index)); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if identity.Name() != "" || identity.ContentID().Available() {
		t.Fatal("captured Identity remained live after Commit")
	}
	if _, ok := order.BodyLen(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("captured Order remained live after Commit")
	}
	if _, ok := binds.Len(keyspace.MakeTerm(keyspace.FamilyBind, 1)); ok {
		t.Fatal("captured Binds remained live after Commit")
	}
	if _, ok := formals.Len(keyspace.MakeTerm(keyspace.FamilyFunction, 1)); ok {
		t.Fatal("captured Formals remained live after Commit")
	}
	if keys.Count() != 0 || keys.ExactCount() != 0 {
		t.Fatal("captured Keys remained live after Commit")
	}
	if faults.Count() != 0 || nils.Count() != 0 {
		t.Fatal("captured Faults or nested Literals remained live after Commit")
	}
	if _, _, ok := nils.At(0); ok {
		t.Fatal("captured literal row remained live after Commit")
	}
}

func TestPreimageNestedLiteralsExpireAfterAbort(t *testing.T) {
	input, _ := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	nils := finalizer.Preimage().Literals().Nils()
	if got := nils.Count(); got != 1 {
		t.Fatalf("live nested literal Count() = %d, want 1", got)
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if got := nils.Count(); got != 0 {
		t.Fatalf("post-Abort nested literal Count() = %d, want 0", got)
	}
	if _, _, ok := nils.At(0); ok {
		t.Fatal("post-Abort nested literal row remained live")
	}
}

func TestPreimageCopiedFinalizersShareFence(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	copy := finalizer
	first, second := finalizer.Preimage(), copy.Preimage()
	if _, err := copy.Commit(ownedIndex(draft, index)); err != nil {
		t.Fatalf("copied Commit: %v", err)
	}
	if first.Identity().Name() != "" || second.Identity().Name() != "" {
		t.Fatal("copied Finalizer Preimages remained live after terminal Commit")
	}
	if err := finalizer.Abort(); err == nil {
		t.Fatal("original Finalizer accepted Abort after copied Commit")
	}
}

func TestPreimageIndependentDraftsDoNotShareFence(t *testing.T) {
	input, firstIndex := sourceFixture(1)
	secondInput, secondIndex := sourceFixture(1)
	firstDraft, err := Build(input)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	secondDraft, err := Build(secondInput)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	firstFinalizer, err := firstDraft.Finalizer()
	if err != nil {
		t.Fatalf("first Finalizer: %v", err)
	}
	secondFinalizer, err := secondDraft.Finalizer()
	if err != nil {
		t.Fatalf("second Finalizer: %v", err)
	}
	second := secondFinalizer.Preimage()
	if _, err := firstFinalizer.Commit(ownedIndex(firstDraft, firstIndex)); err != nil {
		t.Fatalf("first Commit: %v", err)
	}
	if got := second.Identity().Name(); got != secondInput.Name {
		t.Fatalf("second draft expired when first committed: Name = %q", got)
	}
	if _, err := secondFinalizer.Commit(ownedIndex(secondDraft, secondIndex)); err != nil {
		t.Fatalf("second Commit: %v", err)
	}
}

func TestPreimageQueriesRaceTerminalTransition(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	preimage := finalizer.Preimage()
	start := make(chan struct{})
	var group sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for iteration := 0; iteration < 200; iteration++ {
				_ = preimage.Identity().Name()
				_, _ = preimage.Order().BodyLen(keyspace.MakeTerm(keyspace.FamilyBody, 1))
				_, _ = preimage.Binds().Len(keyspace.MakeTerm(keyspace.FamilyBind, 1))
				_, _ = preimage.Formals().Len(keyspace.MakeTerm(keyspace.FamilyFunction, 1))
				_ = preimage.Keys().Count()
				_ = preimage.Faults().Count()
				_ = preimage.Literals().Nils().Count()
			}
		}()
	}
	close(start)
	if _, err := finalizer.Commit(ownedIndex(draft, index)); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	group.Wait()
}

func TestPreimageQueriesAllocateNothing(t *testing.T) {
	input, _ := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	preimage := finalizer.Preimage()
	var name string
	var bodyLen, bindLen, formalLen, keyCount, faultCount, literalCount int
	allocs := testing.AllocsPerRun(1000, func() {
		name = preimage.Identity().Name()
		bodyLen, _ = preimage.Order().BodyLen(keyspace.MakeTerm(keyspace.FamilyBody, 1))
		bindLen, _ = preimage.Binds().Len(keyspace.MakeTerm(keyspace.FamilyBind, 1))
		formalLen, _ = preimage.Formals().Len(keyspace.MakeTerm(keyspace.FamilyFunction, 1))
		keyCount = preimage.Keys().Count()
		faultCount = preimage.Faults().Count()
		literalCount = preimage.Literals().Nils().Count()
	})
	if name == "" || bodyLen < 0 || bindLen < 0 || formalLen < 0 || keyCount < 0 || faultCount < 0 || literalCount < 0 {
		t.Fatal("Preimage query sink was not populated")
	}
	if allocs != 0 {
		t.Fatalf("Preimage queries allocated %v times/run", allocs)
	}
	if got := finalizer.Preimage().Identity().Name(); got != input.Name {
		t.Fatalf("reissued Preimage Name = %q, want %q", got, input.Name)
	}
}
