package program_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower"
)

const transformerValueStorageFixture = `
type Coerce = number
local left, right = 1, 2
local nilValue, boolValue, floatValue, stringValue = nil, true, 1.5, "literal"
local openLeft, openRight = external()
left, right = right, left
local seen = left
return Coerce(seen), right, openLeft, openRight, nilValue, boolValue, floatValue, stringValue
`

func TestTransformerValueSourcesFollowExactProgramDenominators(t *testing.T) {
	left := lowerTransformerValueStorageFixture(t, "transformer-value-source.lua")
	replay := lowerTransformerValueStorageFixture(t, "transformer-value-source.lua")
	foreign := lowerTransformerValueStorageFixture(t, "transformer-value-source-foreign.lua")
	input, replayInput, foreignInput := left.TransformerInput(), replay.TransformerInput(), foreign.TransformerInput()

	literals := left.Source().Literals()
	literalPlanes := []struct {
		name      string
		wantCount int
		count     func() int
		at        func(int) (program.ValueSourceOccurrence, bool)
		replayAt  func(int) (program.ValueSourceOccurrence, bool)
		foreignAt func(int) (program.ValueSourceOccurrence, bool)
		sourceAt  func(int) (keyspace.Term, keyspace.Term, bool)
	}{
		{"Nil", literals.Nils().Count(), input.NilSourceCount, input.NilSourceAt, replayInput.NilSourceAt, foreignInput.NilSourceAt,
			func(index int) (keyspace.Term, keyspace.Term, bool) { return literals.Nils().At(index) }},
		{"Bool", literals.Bools().Count(), input.BoolSourceCount, input.BoolSourceAt, replayInput.BoolSourceAt, foreignInput.BoolSourceAt,
			func(index int) (keyspace.Term, keyspace.Term, bool) {
				term, owner, _, ok := literals.Bools().At(index)
				return term, owner, ok
			}},
		{"Integer", literals.Integers().Count(), input.IntegerSourceCount, input.IntegerSourceAt, replayInput.IntegerSourceAt, foreignInput.IntegerSourceAt,
			func(index int) (keyspace.Term, keyspace.Term, bool) {
				term, owner, _, ok := literals.Integers().At(index)
				return term, owner, ok
			}},
		{"Float", literals.Floats().Count(), input.FloatSourceCount, input.FloatSourceAt, replayInput.FloatSourceAt, foreignInput.FloatSourceAt,
			func(index int) (keyspace.Term, keyspace.Term, bool) {
				term, owner, _, ok := literals.Floats().At(index)
				return term, owner, ok
			}},
		{"String", literals.Strings().Count(), input.StringSourceCount, input.StringSourceAt, replayInput.StringSourceAt, foreignInput.StringSourceAt,
			func(index int) (keyspace.Term, keyspace.Term, bool) {
				term, owner, _, ok := literals.Strings().At(index)
				return term, owner, ok
			}},
	}
	seen := make(map[keyspace.ContentID]string)
	sawLiteral := false
	for _, plane := range literalPlanes {
		if got := plane.count(); got != plane.wantCount {
			t.Fatalf("%sSourceCount = %d, want exact Source plane denominator %d", plane.name, got, plane.wantCount)
		}
		if plane.wantCount == 0 {
			t.Fatalf("fixture did not lower a %s literal source", plane.name)
		}
		for cursor := 0; cursor < plane.wantCount; cursor++ {
			sawLiteral = true
			term, owner, termOK := plane.sourceAt(cursor)
			got, gotOK := plane.at(cursor)
			body, bodyOK := input.Body(owner)
			positioned, positionedOK := input.ContainingBody(term)
			anchor, anchorOK := input.ValueSourceAnchor(term)
			finish, finishOK := anchor.Finish()
			gotBody, gotBodyOK := got.Body()
			gotFinish, gotFinishOK := got.Finish()
			if !termOK || !gotOK || !bodyOK || !positionedOK || !body.Equal(positioned) || !anchorOK || !finishOK ||
				!gotBodyOK || !gotFinishOK || !gotBody.Equal(body) || !gotFinish.Equal(finish) {
				t.Fatalf("%sSourceAt(%d) did not preserve its Source plane order and issued Body/anchor", plane.name, cursor)
			}
			assertUniqueTransformerRoleID(t, seen, got.ContextID(), plane.name+"-literal")
			if !input.OwnsValueSourceOccurrence(got) || !input.OwnsValueSourceAnchor(anchor) {
				t.Fatalf("exact input rejected %sSourceAt(%d) or its anchor", plane.name, cursor)
			}
			replayed, replayedOK := plane.replayAt(cursor)
			replayedAnchor, replayedAnchorOK := replayInput.ValueSourceAnchor(term)
			if !replayedOK || !replayedAnchorOK || replayed.ContextID() != got.ContextID() || replayedAnchor.ContextID() != anchor.ContextID() ||
				input.OwnsValueSourceOccurrence(replayed) || input.OwnsValueSourceAnchor(replayedAnchor) {
				t.Fatalf("%sSourceAt(%d) replay identity/ownership law failed", plane.name, cursor)
			}
			other, otherOK := plane.foreignAt(cursor)
			if !otherOK || other.ContextID() == got.ContextID() || input.OwnsValueSourceOccurrence(other) {
				t.Fatalf("%sSourceAt(%d) foreign identity/ownership law failed", plane.name, cursor)
			}
		}
		if _, ok := plane.at(-1); ok {
			t.Fatalf("%sSourceAt accepted a negative cursor", plane.name)
		}
		if _, ok := plane.at(plane.wantCount); ok {
			t.Fatalf("%sSourceAt accepted its denominator", plane.name)
		}
	}
	if !sawLiteral {
		t.Fatal("fixture did not lower a literal source")
	}

	typeValues := left.Flow().Authored().TypeValues()
	if got := input.TypeValueSourceCount(); got != typeValues.Count() {
		t.Fatalf("TypeValueSourceCount = %d, want authored Program candidate denominator %d", got, typeValues.Count())
	}
	if typeValues.Count() == 0 {
		t.Fatal("fixture did not lower a Program-local TypeValue candidate")
	}
	for cursor := 0; cursor < typeValues.Count(); cursor++ {
		term, termOK := typeValues.At(cursor)
		owner, rowOK := typeValues.Get(term)
		target, targetOK := left.Static().Operands().TypeValues().Target(term)
		ref, refOK := left.Static().StaticTypes().Ref(target)
		want := termOK && rowOK && targetOK && refOK && ref.Term() == target && left.Flow().Executable().Contains(term)
		got, gotOK := input.TypeValueSourceAt(cursor)
		if gotOK != want {
			t.Fatalf("TypeValueSourceAt(%d) availability = %v, want executable Program candidate %v", cursor, gotOK, want)
		}
		if !want {
			continue
		}
		body, bodyOK := input.Body(owner)
		positioned, positionedOK := input.ContainingBody(term)
		anchor, anchorOK := input.ValueSourceAnchor(term)
		finish, finishOK := anchor.Finish()
		gotBody, gotBodyOK := got.Body()
		gotFinish, gotFinishOK := got.Finish()
		if !bodyOK || !positionedOK || !body.Equal(positioned) || !anchorOK || !finishOK || !gotBodyOK || !gotFinishOK ||
			!gotBody.Equal(body) || !gotFinish.Equal(finish) {
			t.Fatalf("TypeValueSourceAt(%d) did not preserve authored order and exact Body/Finish", cursor)
		}
		assertUniqueTransformerRoleID(t, seen, got.ContextID(), "type-value")
		if !input.OwnsValueSourceOccurrence(got) || !input.OwnsValueSourceAnchor(anchor) {
			t.Fatalf("exact input rejected TypeValueSourceAt(%d)", cursor)
		}
		replayed, replayedOK := replayInput.TypeValueSourceAt(cursor)
		if !replayedOK || replayed.ContextID() != got.ContextID() || input.OwnsValueSourceOccurrence(replayed) {
			t.Fatalf("TypeValueSourceAt(%d) replay identity/ownership law failed", cursor)
		}
		other, otherOK := foreignInput.TypeValueSourceAt(cursor)
		if !otherOK || other.ContextID() == got.ContextID() || input.OwnsValueSourceOccurrence(other) {
			t.Fatalf("TypeValueSourceAt(%d) foreign identity/ownership law failed", cursor)
		}
	}
	if _, ok := input.TypeValueSourceAt(-1); ok {
		t.Fatal("TypeValueSourceAt accepted a negative cursor")
	}
	if _, ok := input.TypeValueSourceAt(typeValues.Count()); ok {
		t.Fatal("TypeValueSourceAt accepted its denominator")
	}
	entry, entryOK := left.Source().Index().Entry()
	if !entryOK {
		t.Fatal("fixture has no Program entry")
	}
	if _, ok := input.ValueSourceAnchor(entry); ok {
		t.Fatal("ValueSourceAnchor accepted a non-value-source Body")
	}
}

func TestTransformerValueSourceKeepsRepeatConditionExecutionOwner(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "transformer-value-source-repeat.lua", Text: []byte(`repeat local x = 1 until true`)})
	if err != nil {
		t.Fatal(err)
	}
	input := p.TransformerInput()
	term, owner, _, rowOK := p.Source().Literals().Bools().At(0)
	source, sourceOK := input.BoolSourceAt(0)
	body, bodyOK := input.Body(owner)
	containing, containingOK := input.ContainingBody(term)
	sourceBody, sourceBodyOK := source.Body()
	finish, finishOK := source.Finish()
	if !rowOK || !sourceOK || !bodyOK || !containingOK || body.Equal(containing) || !sourceBodyOK ||
		!sourceBody.Equal(body) || !finishOK || !input.OwnsSite(finish) || !input.OwnsValueSourceOccurrence(source) {
		t.Fatal("Repeat condition source did not preserve its distinct Source-owned execution Body")
	}
}

func TestTransformerStorageReadKeepsRepeatConditionExecutionOwner(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "transformer-storage-read-repeat.lua", Text: []byte(`local open = false repeat open = true until open`)})
	if err != nil {
		t.Fatal(err)
	}
	input := p.TransformerInput()
	reads := p.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		term, termOK := reads.At(index)
		owner, _, _, rowOK := reads.Get(term)
		body, bodyOK := input.Body(owner)
		containing, containingOK := input.ContainingBody(term)
		if !termOK || !rowOK || !bodyOK || !containingOK || body.Equal(containing) {
			continue
		}
		read, readOK := input.StorageReadAt(index)
		readBody, readBodyOK := read.Body()
		if !readOK || !readBodyOK || !readBody.Equal(body) || !input.OwnsStorageReadOccurrence(read) {
			t.Fatal("Repeat condition Read did not preserve Flow's distinct execution Body")
		}
		return
	}
	t.Fatal("fixture did not expose a Repeat condition Read with distinct syntactic containment")
}

func TestTransformerStorageRolesFollowExactOrderedOwners(t *testing.T) {
	left := lowerTransformerValueStorageFixture(t, "transformer-storage-role.lua")
	replay := lowerTransformerValueStorageFixture(t, "transformer-storage-role.lua")
	foreign := lowerTransformerValueStorageFixture(t, "transformer-storage-role-foreign.lua")
	input, replayInput, foreignInput := left.TransformerInput(), replay.TransformerInput(), foreign.TransformerInput()
	storage := left.Flow().Authored().Storage()
	seen := make(map[keyspace.ContentID]string)

	reads := storage.Reads()
	if got := input.StorageReadCount(); got != reads.Count() {
		t.Fatalf("StorageReadCount = %d, want authored Read denominator %d", got, reads.Count())
	}
	if reads.Count() == 0 {
		t.Fatal("fixture did not lower a storage Read")
	}
	for cursor := 0; cursor < reads.Count(); cursor++ {
		term, termOK := reads.At(cursor)
		owner, source, _, rowOK := reads.Get(term)
		_, _, _, cellOK := storage.Cells().Get(source)
		span, spanOK := input.Span(term)
		body, bodyOK := input.Body(owner)
		positioned, positionedOK := input.ContainingBody(term)
		want := termOK && rowOK && cellOK && spanOK && bodyOK && positionedOK && body.Equal(positioned) &&
			left.Flow().Executable().Contains(term)
		got, gotOK := input.StorageReadAt(cursor)
		if gotOK != want {
			t.Fatalf("StorageReadAt(%d) availability = %v, want exact authored candidate %v", cursor, gotOK, want)
		}
		if !want {
			continue
		}
		assertBodyAndSpan(t, "StorageReadAt", cursor, got.Body, got.Entry, got.Finish, body, span)
		assertUniqueTransformerRoleID(t, seen, got.ContextID(), "read")
		if !input.OwnsStorageReadOccurrence(got) {
			t.Fatalf("exact input rejected StorageReadAt(%d)", cursor)
		}
		replayed, replayedOK := replayInput.StorageReadAt(cursor)
		if !replayedOK || replayed.ContextID() != got.ContextID() || input.OwnsStorageReadOccurrence(replayed) {
			t.Fatalf("StorageReadAt(%d) replay identity/ownership law failed", cursor)
		}
		other, otherOK := foreignInput.StorageReadAt(cursor)
		if !otherOK || other.ContextID() == got.ContextID() || input.OwnsStorageReadOccurrence(other) {
			t.Fatalf("StorageReadAt(%d) foreign identity/ownership law failed", cursor)
		}
	}
	if _, ok := input.StorageReadAt(-1); ok {
		t.Fatal("StorageReadAt accepted a negative cursor")
	}
	if _, ok := input.StorageReadAt(reads.Count()); ok {
		t.Fatal("StorageReadAt accepted its denominator")
	}

	binds := storage.Binds()
	if got := input.StorageBindCount(); got != binds.Count() {
		t.Fatalf("StorageBindCount = %d, want authored Bind denominator %d", got, binds.Count())
	}
	if binds.Count() == 0 {
		t.Fatal("fixture did not lower a storage Bind")
	}
	sawBindTransfer := false
	for cursor := 0; cursor < binds.Count(); cursor++ {
		term, termOK := binds.At(cursor)
		owner, values, rowOK := binds.Get(term)
		width, widthOK := left.Source().Binds().Len(term)
		_, _, valuesOK := left.Flow().Authored().Values().Get(values)
		span, spanOK := input.Span(term)
		body, bodyOK := input.Body(owner)
		positioned, positionedOK := input.ContainingBody(term)
		want := termOK && rowOK && widthOK && valuesOK && spanOK && bodyOK && positionedOK && body.Equal(positioned) &&
			left.Flow().Executable().Contains(term)
		got, gotOK := input.StorageBindAt(cursor)
		if gotOK != want {
			t.Fatalf("StorageBindAt(%d) availability = %v, want exact authored candidate %v", cursor, gotOK, want)
		}
		if !want {
			continue
		}
		if got.TransferCount() != width {
			t.Fatalf("StorageBindAt(%d).TransferCount = %d, want Source.Bind width %d", cursor, got.TransferCount(), width)
		}
		assertBodyAndSpan(t, "StorageBindAt", cursor, got.Body, got.Entry, got.Finish, body, span)
		assertUniqueTransformerRoleID(t, seen, got.ContextID(), "bind")
		if !input.OwnsStorageBind(got) {
			t.Fatalf("exact input rejected StorageBindAt(%d)", cursor)
		}
		replayed, replayedOK := replayInput.StorageBindAt(cursor)
		if !replayedOK || replayed.ContextID() != got.ContextID() || input.OwnsStorageBind(replayed) {
			t.Fatalf("StorageBindAt(%d) replay identity/ownership law failed", cursor)
		}
		other, otherOK := foreignInput.StorageBindAt(cursor)
		if !otherOK || other.ContextID() == got.ContextID() || input.OwnsStorageBind(other) {
			t.Fatalf("StorageBindAt(%d) foreign identity/ownership law failed", cursor)
		}
		for position := 0; position < width; position++ {
			cell, bound := left.Source().Binds().At(term, position)
			_, fixed := left.Flow().Authored().Values().Member(values, position)
			_, _, _, cellOK := storage.Cells().Get(cell)
			boundCell, boundCellOK := got.CellAt(position)
			if boundCellOK != (bound && cellOK) {
				t.Fatalf("StorageBindAt(%d).CellAt(%d) availability = %v, want exact destination cell %v", cursor, position, boundCellOK, bound && cellOK)
			}
			if boundCellOK {
				replayCell, replayCellOK := replayed.CellAt(position)
				otherCell, otherCellOK := other.CellAt(position)
				if !replayCellOK || replayCell.ContextID() != boundCell.ContextID() || !otherCellOK || otherCell.ContextID() == boundCell.ContextID() {
					t.Fatalf("StorageBindAt(%d).CellAt(%d) replay/foreign identity law failed", cursor, position)
				}
			}
			wantTransfer := bound && fixed && cellOK
			transfer, transferOK := got.TransferAt(position)
			if transferOK != wantTransfer {
				t.Fatalf("StorageBindAt(%d).TransferAt(%d) availability = %v, want exact ordered transfer %v", cursor, position, transferOK, wantTransfer)
			}
			if !wantTransfer {
				continue
			}
			sawBindTransfer = true
			assertBodyAndSpan(t, "StorageBind.TransferAt", position, transfer.Body, transfer.Entry, transfer.Finish, body, span)
			assertUniqueTransformerRoleID(t, seen, transfer.ContextID(), "bind-transfer")
			if !input.OwnsStorageBindOccurrence(transfer) {
				t.Fatalf("exact input rejected StorageBindAt(%d).TransferAt(%d)", cursor, position)
			}
			replayedTransfer, replayedTransferOK := replayed.TransferAt(position)
			if !replayedTransferOK || replayedTransfer.ContextID() != transfer.ContextID() || input.OwnsStorageBindOccurrence(replayedTransfer) {
				t.Fatalf("StorageBindAt(%d).TransferAt(%d) replay identity/ownership law failed", cursor, position)
			}
			otherTransfer, otherTransferOK := other.TransferAt(position)
			if !otherTransferOK || otherTransfer.ContextID() == transfer.ContextID() || input.OwnsStorageBindOccurrence(otherTransfer) {
				t.Fatalf("StorageBindAt(%d).TransferAt(%d) foreign identity/ownership law failed", cursor, position)
			}
		}
		if _, ok := got.TransferAt(-1); ok {
			t.Fatalf("StorageBindAt(%d).TransferAt accepted a negative position", cursor)
		}
		if _, ok := got.TransferAt(width); ok {
			t.Fatalf("StorageBindAt(%d).TransferAt accepted its denominator", cursor)
		}
		if _, ok := got.CellAt(-1); ok {
			t.Fatalf("StorageBindAt(%d).CellAt accepted a negative position", cursor)
		}
		if _, ok := got.CellAt(width); ok {
			t.Fatalf("StorageBindAt(%d).CellAt accepted its denominator", cursor)
		}
	}
	if !sawBindTransfer {
		t.Fatal("fixture did not issue an exact fixed Bind transfer")
	}

	assigns := storage.Assigns()
	if got := input.StorageAssignmentCount(); got != assigns.Count() {
		t.Fatalf("StorageAssignmentCount = %d, want authored Assign denominator %d", got, assigns.Count())
	}
	if assigns.Count() == 0 {
		t.Fatal("fixture did not lower a storage Assign")
	}
	sawWriteTransfer := false
	for cursor := 0; cursor < assigns.Count(); cursor++ {
		term, termOK := assigns.At(cursor)
		owner, values, rowOK := assigns.Get(term)
		width, widthOK := assigns.WriteCount(term)
		_, _, valuesOK := left.Flow().Authored().Values().Get(values)
		body, bodyOK := input.Body(owner)
		positioned, positionedOK := input.ContainingBody(term)
		want := termOK && rowOK && widthOK && width > 0 && valuesOK && bodyOK && positionedOK && body.Equal(positioned) &&
			left.Flow().Executable().Contains(term)
		got, gotOK := input.StorageAssignmentAt(cursor)
		if gotOK != want {
			t.Fatalf("StorageAssignmentAt(%d) availability = %v, want executable Assign candidate %v", cursor, gotOK, want)
		}
		if !want {
			continue
		}
		if got.TransferCount() != width {
			t.Fatalf("StorageAssignmentAt(%d).TransferCount = %d, want Assign.WriteCount %d", cursor, got.TransferCount(), width)
		}
		gotBody, gotBodyOK := got.Body()
		if !gotBodyOK || !gotBody.Equal(body) {
			t.Fatalf("StorageAssignmentAt(%d) did not retain its exact Body", cursor)
		}
		assertUniqueTransformerRoleID(t, seen, got.ContextID(), "assignment")
		if !input.OwnsStorageAssignment(got) {
			t.Fatalf("exact input rejected StorageAssignmentAt(%d)", cursor)
		}
		replayed, replayedOK := replayInput.StorageAssignmentAt(cursor)
		if !replayedOK || replayed.ContextID() != got.ContextID() || input.OwnsStorageAssignment(replayed) {
			t.Fatalf("StorageAssignmentAt(%d) replay identity/ownership law failed", cursor)
		}
		other, otherOK := foreignInput.StorageAssignmentAt(cursor)
		if !otherOK || other.ContextID() == got.ContextID() || input.OwnsStorageAssignment(other) {
			t.Fatalf("StorageAssignmentAt(%d) foreign identity/ownership law failed", cursor)
		}
		for position := 0; position < width; position++ {
			writeTerm, writeOK := assigns.WriteAt(term, position)
			actualAssign, target, rowOK := storage.Writes().Get(writeTerm)
			_, fixed := left.Flow().Authored().Values().Member(values, position)
			_, _, _, cellOK := storage.Cells().Get(target)
			finishTerm, finishOK := left.Flow().Ports().Finish(writeTerm)
			finish, finishSiteOK := left.Flow().Causal().Sites().ForTerm(finishTerm)
			successor, predecessorOK := left.Flow().Causal().Successors().AssignmentPredecessor(writeTerm)
			identity, identityOK := successor.Identity()
			wantTransfer := writeOK && rowOK && actualAssign == term && fixed && cellOK && finishOK && finishSiteOK &&
				predecessorOK && identityOK && successor.To == finishTerm && successor.Arm == flow.BoundaryLocal &&
				identity.To() == finishTerm && identity.Arm() == flow.BoundaryLocal && identity.Provenance() == left.Flow().Provenance()
			write, gotWrite := got.TransferAt(position)
			if gotWrite != wantTransfer {
				t.Fatalf("StorageAssignmentAt(%d).TransferAt(%d) availability = %v, want exact ordered Write %v", cursor, position, gotWrite, wantTransfer)
			}
			if !wantTransfer {
				continue
			}
			sawWriteTransfer = true
			gotFinish, gotFinishOK := write.Finish()
			predecessor, gotPredecessor := write.Predecessor()
			digest, digestOK := predecessor.RouteDigest()
			if !gotFinishOK || !gotFinish.Equal(finish) || !gotPredecessor || !digestOK || digest != identity.Digest() {
				t.Fatalf("StorageAssignmentAt(%d).TransferAt(%d) did not preserve Finish/predecessor route", cursor, position)
			}
			assertUniqueTransformerRoleID(t, seen, write.ContextID(), "write-transfer")
			assertUniqueTransformerRoleID(t, seen, predecessor.ContextID(), "assignment-predecessor")
			if !input.OwnsStorageWriteOccurrence(write) || !input.OwnsAssignmentPredecessor(predecessor) {
				t.Fatalf("exact input rejected StorageAssignmentAt(%d).TransferAt(%d)", cursor, position)
			}
			replayedWrite, replayedWriteOK := replayed.TransferAt(position)
			replayedPredecessor, replayedPredecessorOK := replayedWrite.Predecessor()
			if !replayedWriteOK || !replayedPredecessorOK || replayedWrite.ContextID() != write.ContextID() ||
				replayedPredecessor.ContextID() != predecessor.ContextID() || input.OwnsStorageWriteOccurrence(replayedWrite) ||
				input.OwnsAssignmentPredecessor(replayedPredecessor) {
				t.Fatalf("StorageAssignmentAt(%d).TransferAt(%d) replay identity/ownership law failed", cursor, position)
			}
			otherWrite, otherWriteOK := other.TransferAt(position)
			otherPredecessor, otherPredecessorOK := otherWrite.Predecessor()
			if !otherWriteOK || !otherPredecessorOK || otherWrite.ContextID() == write.ContextID() ||
				otherPredecessor.ContextID() == predecessor.ContextID() || input.OwnsStorageWriteOccurrence(otherWrite) ||
				input.OwnsAssignmentPredecessor(otherPredecessor) {
				t.Fatalf("StorageAssignmentAt(%d).TransferAt(%d) foreign identity/ownership law failed", cursor, position)
			}
		}
		if _, ok := got.TransferAt(-1); ok {
			t.Fatalf("StorageAssignmentAt(%d).TransferAt accepted a negative position", cursor)
		}
		if _, ok := got.TransferAt(width); ok {
			t.Fatalf("StorageAssignmentAt(%d).TransferAt accepted its denominator", cursor)
		}
	}
	if !sawWriteTransfer {
		t.Fatal("fixture did not issue an exact fixed Write transfer")
	}
}

func TestTransformerValueStorageRolesFailClosedAndHideRawCoordinates(t *testing.T) {
	var input program.TransformerInput
	if input.NilSourceCount() != 0 || input.BoolSourceCount() != 0 || input.IntegerSourceCount() != 0 ||
		input.FloatSourceCount() != 0 || input.StringSourceCount() != 0 || input.TypeValueSourceCount() != 0 ||
		input.StorageReadCount() != 0 || input.StorageBindCount() != 0 || input.StorageAssignmentCount() != 0 {
		t.Fatal("zero TransformerInput exposed a value/storage denominator")
	}
	for name, at := range map[string]func(int) (program.ValueSourceOccurrence, bool){
		"Nil": input.NilSourceAt, "Bool": input.BoolSourceAt, "Integer": input.IntegerSourceAt,
		"Float": input.FloatSourceAt, "String": input.StringSourceAt,
	} {
		if _, ok := at(0); ok {
			t.Fatalf("zero TransformerInput exposed a %s source", name)
		}
	}
	if _, ok := input.TypeValueSourceAt(0); ok {
		t.Fatal("zero TransformerInput exposed a TypeValue source")
	}
	if _, ok := input.StorageReadAt(0); ok {
		t.Fatal("zero TransformerInput exposed a Read")
	}
	if _, ok := input.StorageBindAt(0); ok {
		t.Fatal("zero TransformerInput exposed a Bind")
	}
	if _, ok := input.StorageAssignmentAt(0); ok {
		t.Fatal("zero TransformerInput exposed an Assign")
	}
	if _, ok := input.ValueSourceAnchor(keyspace.MakeTerm(keyspace.FamilyInteger, 1)); ok {
		t.Fatal("zero TransformerInput exposed a ValueSourceAnchor")
	}

	zeroRoles := []struct {
		name      string
		available bool
		id        keyspace.ContentID
	}{
		{"ValueSourceAnchor", (program.ValueSourceAnchor{}).Available(), (program.ValueSourceAnchor{}).ContextID()},
		{"ValueSourceOccurrence", (program.ValueSourceOccurrence{}).Available(), (program.ValueSourceOccurrence{}).ContextID()},
		{"StorageReadOccurrence", (program.StorageReadOccurrence{}).Available(), (program.StorageReadOccurrence{}).ContextID()},
		{"StorageBind", (program.StorageBind{}).Available(), (program.StorageBind{}).ContextID()},
		{"StorageBindOccurrence", (program.StorageBindOccurrence{}).Available(), (program.StorageBindOccurrence{}).ContextID()},
		{"StorageAssignment", (program.StorageAssignment{}).Available(), (program.StorageAssignment{}).ContextID()},
		{"StorageWriteOccurrence", (program.StorageWriteOccurrence{}).Available(), (program.StorageWriteOccurrence{}).ContextID()},
		{"AssignmentPredecessor", (program.AssignmentPredecessor{}).Available(), (program.AssignmentPredecessor{}).ContextID()},
	}
	for _, role := range zeroRoles {
		if role.available || role.id.Available() {
			t.Fatalf("zero %s did not fail closed", role.name)
		}
	}
	if (program.StorageBind{}).TransferCount() != 0 || (program.StorageAssignment{}).TransferCount() != 0 {
		t.Fatal("zero Bind/Assign exposed a transfer denominator")
	}
	if _, ok := (program.StorageBind{}).TransferAt(0); ok {
		t.Fatal("zero Bind exposed a transfer")
	}
	if _, ok := (program.StorageAssignment{}).TransferAt(0); ok {
		t.Fatal("zero Assign exposed a transfer")
	}

	termType := reflect.TypeOf(keyspace.Term(0))
	roleTypes := []reflect.Type{
		reflect.TypeOf(program.ValueSourceAnchor{}), reflect.TypeOf(program.ValueSourceOccurrence{}), reflect.TypeOf(program.StorageReadOccurrence{}),
		reflect.TypeOf(program.StorageBind{}), reflect.TypeOf(program.StorageBindOccurrence{}),
		reflect.TypeOf(program.StorageAssignment{}), reflect.TypeOf(program.StorageWriteOccurrence{}),
		reflect.TypeOf(program.AssignmentPredecessor{}),
	}
	for _, roleType := range roleTypes {
		for methodIndex := 0; methodIndex < roleType.NumMethod(); methodIndex++ {
			method := roleType.Method(methodIndex)
			for resultIndex := 0; resultIndex < method.Type.NumOut(); resultIndex++ {
				if method.Type.Out(resultIndex) == termType {
					t.Fatalf("%s.%s publicly returns raw keyspace.Term", roleType.Name(), method.Name)
				}
			}
		}
	}
	if _, ok := reflect.TypeOf(program.StorageWriteOccurrence{}).MethodByName("Entry"); ok {
		t.Fatal("StorageWriteOccurrence fabricated an Entry coordinate")
	}
}

func lowerTransformerValueStorageFixture(t *testing.T, name string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: name, Text: []byte(transformerValueStorageFixture)})
	if err != nil {
		t.Fatalf("lower %s: %v", name, err)
	}
	return p
}

func assertBodyAndSpan(
	t *testing.T,
	name string,
	index int,
	bodyAt func() (program.Body, bool),
	entryAt func() (flow.Site, bool),
	finishAt func() (flow.Site, bool),
	wantBody program.Body,
	wantSpan program.Span,
) {
	t.Helper()
	body, bodyOK := bodyAt()
	entry, entryOK := entryAt()
	finish, finishOK := finishAt()
	wantEntry, wantEntryOK := wantSpan.Entry()
	wantFinish, wantFinishOK := wantSpan.Finish()
	if !bodyOK || !entryOK || !finishOK || !wantEntryOK || !wantFinishOK || !body.Equal(wantBody) ||
		!entry.Equal(wantEntry) || !finish.Equal(wantFinish) {
		t.Fatalf("%s(%d) did not preserve exact Body and Entry/Finish", name, index)
	}
}

func assertUniqueTransformerRoleID(t *testing.T, seen map[keyspace.ContentID]string, id keyspace.ContentID, kind string) {
	t.Helper()
	if !id.Available() {
		t.Fatalf("%s role has no ContextID", kind)
	}
	if previous, duplicate := seen[id]; duplicate {
		t.Fatalf("%s role duplicated %s ContextID", kind, previous)
	}
	seen[id] = kind
}
