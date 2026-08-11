package value

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestStorageTransferFixtureRetainsStructurallyValidDeadBind(t *testing.T) {
	schema, linked := storageTransferSchema(t)
	mounts := linked.Project().Mounts()
	var deadShard linkproject.Shard
	var deadBind keyspace.Term
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			t.Fatalf("ShardAt(%d)", index)
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil {
			t.Fatalf("Program(%v)", shard)
		}
		binds := p.Flow().Authored().Storage().Binds()
		bindOrder := p.Source().Binds()
		for bindIndex := 0; bindIndex < binds.Count(); bindIndex++ {
			bind, present := binds.At(bindIndex)
			owner, valuePack, related := binds.Get(bind)
			width, sized := bindOrder.Len(bind)
			if !present || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 || valuePack == 0 || !related || !sized || width <= 0 {
				t.Fatalf("Bind(%v) structural owner data = present:%v owner:%v values:%v related:%v sized:%v width:%d", bind, present, owner, valuePack, related, sized, width)
			}
			if p.Flow().Executable().Contains(bind) {
				continue
			}
			deadShard, deadBind = shard, bind
			break
		}
		if deadBind != 0 {
			break
		}
	}
	if deadBind == 0 {
		t.Fatal("fixture omitted a structurally valid non-executable Bind")
	}
	for index := 0; index < schema.StorageTransferCount(); index++ {
		transfer, ok := schema.StorageTransferAt(index)
		if !ok {
			t.Fatalf("StorageTransferAt(%d)", index)
		}
		ref, ok := transfer.Ref()
		if ok && ref.shard == deadShard && ref.kind == storageTransferBind && ref.term == deadBind {
			t.Fatalf("dead Bind %v entered Value storage-transfer denominator", deadBind)
		}
	}
}

func TestStorageTransferFencesOwnerAndRebindsOnlyThroughRef(t *testing.T) {
	leftSchema, leftLink := storageTransferSchema(t)
	rightSchema, rightLink := storageTransferSchema(t)
	if leftLink.ContentID() != rightLink.ContentID() || leftLink == rightLink {
		t.Fatal("same-content Links")
	}

	left, ok := leftSchema.StorageTransferAt(0)
	if !ok {
		t.Fatal("Value storage transfer")
	}
	from, to, endpoints := left.Endpoints()
	if !endpoints || !from.Valid() || !to.Valid() {
		t.Fatal("exact Value endpoints")
	}
	id, identified := left.ID()
	if !identified || !id.Available() {
		t.Fatal("Value-owned storage transfer ID")
	}
	ref, replayable := left.Ref()
	if !replayable {
		t.Fatal("storage transfer ref")
	}
	rebound, reboundOK := leftSchema.FindStorageTransfer(ref)
	if !reboundOK || rebound != left {
		t.Fatal("same-schema transfer rebind")
	}

	right, ok := rightSchema.StorageTransferAt(0)
	if !ok {
		t.Fatal("right storage transfer")
	}
	if leftSchema.OwnsStorageTransfer(right) {
		t.Fatal("foreign Value transfer crossed owner fence")
	}
	right, reboundOK = rightSchema.FindStorageTransfer(ref)
	if !reboundOK {
		t.Fatal("same-content replay transfer")
	}
	rightID, rightIDOK := right.ID()
	if !rightIDOK || rightID != id {
		t.Fatal("rebound transfer identity changed")
	}
	rightFrom, rightTo, rightEndpoints := right.Endpoints()
	if !rightEndpoints || rightFrom == from || rightTo == to {
		t.Fatal("rebound transfer retained foreign Value coordinates")
	}

	if _, ok := leftSchema.FindStorageTransfer(StorageTransferRef{}); ok {
		t.Fatal("zero transfer ref rebound")
	}
}

func TestStorageTransferEndpointsUseOnlyExistingValueCoordinates(t *testing.T) {
	schema, linked := storageTransferSchema(t)
	if schema.StorageTransferCount() < 4 {
		t.Fatalf("storage transfer denominator = %d, want fixed Read/Bind/Write coverage", schema.StorageTransferCount())
	}
	for index := 0; index < schema.StorageTransferCount(); index++ {
		transfer, ok := schema.StorageTransferAt(index)
		if !ok {
			t.Fatalf("StorageTransferAt(%d)", index)
		}
		from, to, ok := transfer.Endpoints()
		if !ok || !from.Valid() || !to.Valid() {
			t.Fatalf("StorageTransfer(%d) endpoints", index)
		}
		if _, ok := schema.CoordinateIndex(from); !ok {
			t.Fatalf("StorageTransfer(%d) fabricated source coordinate", index)
		}
		if _, ok := schema.CoordinateIndex(to); !ok {
			t.Fatalf("StorageTransfer(%d) fabricated destination coordinate", index)
		}
		id, identified := transfer.ID()
		if !identified || !id.Available() {
			t.Fatalf("StorageTransfer(%d) missing identity", index)
		}
		ref, replayable := transfer.Ref()
		if !replayable {
			t.Fatalf("StorageTransfer(%d) missing occurrence proof", index)
		}
		shard, occurrence, occurrenceOK := transfer.Occurrence()
		if !occurrenceOK || shard != ref.shard || occurrence != ref.term {
			t.Fatalf("StorageTransfer(%d) did not retain its exact authored occurrence", index)
		}
		inverse, inverseOK := schema.StorageTransferFor(ref.shard, ref.term, int(ref.position))
		if !inverseOK || inverse != transfer {
			t.Fatalf("StorageTransfer(%d) occurrence inverse", index)
		}
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	foreignSchema, foreign := storageTransferSchema(t)
	foreignShard, foreignShardOK := foreign.Project().Mounts().At(0)
	if !shardOK || !foreignShardOK {
		t.Fatal("fixture shards")
	}
	if _, ok := (*Schema)(nil).StorageTransferFor(shard, keyspace.MakeTerm(keyspace.FamilyRead, 1), 0); ok {
		t.Fatal("nil Schema inverted a storage occurrence")
	}
	if _, ok := schema.StorageTransferFor(foreignShard, keyspace.MakeTerm(keyspace.FamilyRead, 1), 0); ok {
		t.Fatal("foreign Project shard crossed the storage inverse fence")
	}
	if _, ok := foreignSchema.StorageTransferFor(shard, keyspace.MakeTerm(keyspace.FamilyRead, 1), 0); ok {
		t.Fatal("local Project shard crossed the foreign storage inverse fence")
	}
	if _, ok := schema.StorageTransferFor(shard, keyspace.MakeTerm(keyspace.FamilyReturn, 1), 0); ok {
		t.Fatal("non-storage occurrence inverted a transfer")
	}
	if _, ok := schema.StorageTransferFor(shard, keyspace.MakeTerm(keyspace.FamilyRead, 1), 1); ok {
		t.Fatal("Read position invented a transfer")
	}
	var zero StorageTransfer
	if _, _, ok := zero.Occurrence(); ok {
		t.Fatal("zero storage transfer acquired an occurrence")
	}
	transfer, ok := schema.StorageTransferAt(0)
	if !ok {
		t.Fatal("first storage transfer")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _, _ = transfer.Occurrence()
	}); allocations != 0 {
		t.Fatalf("StorageTransfer.Occurrence allocations = %g, want 0", allocations)
	}
}

func TestStorageTransferForRejectsMalformedDeadAndOutOfBounds(t *testing.T) {
	schema, linked := storageTransferSchema(t)
	shard, shardOK := linked.Project().Mounts().At(0)
	if !shardOK {
		t.Fatal("fixture shard")
	}
	transfer, transferOK := schema.StorageTransferAt(0)
	if !transferOK {
		t.Fatal("first storage transfer")
	}
	ref, refOK := transfer.Ref()
	if !refOK {
		t.Fatal("first storage transfer ref")
	}
	if _, ok := schema.StorageTransferFor(ref.shard, ref.term, -1); ok {
		t.Fatal("negative storage position accepted")
	}
	if _, ok := schema.StorageTransferFor(ref.shard, ref.term, int(^uint32(0))); ok {
		t.Fatal("maximum non-bind position accepted")
	}
	if _, ok := schema.StorageTransferFor(shard, keyspace.Term(^uint32(0)), 0); ok {
		t.Fatal("malformed storage family accepted")
	}

	var deadShard linkproject.Shard
	var deadBind keyspace.Term
	mounts := linked.Project().Mounts()
	for index := 0; index < mounts.Count() && deadBind == 0; index++ {
		candidateShard, candidateOK := mounts.At(index)
		p, programOK := mounts.Program(candidateShard)
		if !candidateOK || !programOK || p == nil {
			t.Fatalf("fixture mount %d", index)
		}
		binds := p.Flow().Authored().Storage().Binds()
		for bindIndex := 0; bindIndex < binds.Count(); bindIndex++ {
			bind, present := binds.At(bindIndex)
			if present && !p.Flow().Executable().Contains(bind) {
				deadShard, deadBind = candidateShard, bind
				break
			}
		}
	}
	if deadBind == 0 {
		t.Fatal("fixture omitted a dead Bind")
	}
	if _, ok := schema.StorageTransferFor(deadShard, deadBind, 0); ok {
		t.Fatal("non-executable Bind entered storage inverse")
	}

	allocations := testing.AllocsPerRun(100, func() {
		if _, ok := schema.StorageTransferFor(ref.shard, ref.term, int(ref.position)); !ok {
			t.Fatal("sealed storage inverse disappeared")
		}
	})
	if allocations != 0 {
		t.Fatalf("StorageTransferFor allocated %.1f times", allocations)
	}
}

// The ordinal is opaque outside Value. A forged in-package handle still has
// to name one exact sealed row in its issuing Schema.
func TestStorageTransferRejectsDetachedSchemaOrOrdinal(t *testing.T) {
	schema, _ := storageTransferSchema(t)
	first, ok := schema.StorageTransferAt(0)
	if !ok || !first.valid() {
		t.Fatal("first Value transfer")
	}
	if schema.StorageTransferCount() < 2 {
		t.Fatal("second transfer denominator")
	}
	second, ok := schema.StorageTransferAt(1)
	if !ok {
		t.Fatal("second Value transfer")
	}
	forged := first
	forged.index = second.index
	if !forged.valid() || forged == first {
		t.Fatal("sealed transfer ordinal did not select its exact row")
	}
	foreignSchema, _ := storageTransferSchema(t)
	forged.schema = foreignSchema
	if !forged.valid() || schema.OwnsStorageTransfer(forged) {
		t.Fatal("foreign schema crossed Value transfer owner fence")
	}
}

func TestStorageTransferWideAssignmentKeepsDenseWriteDenominator(t *testing.T) {
	const width = 256
	schema, linked := wideStorageTransferSchema(t, width)
	wantWrites := 0
	for index := 0; index < linked.Project().Mounts().Count(); index++ {
		shard, ok := linked.Project().Mounts().At(index)
		if !ok {
			t.Fatalf("ShardAt(%d)", index)
		}
		p, ok := linked.Project().Mounts().Program(shard)
		if !ok || p == nil {
			t.Fatalf("Program(%v)", shard)
		}
		assigns := p.Flow().Authored().Storage().Assigns()
		for assignIndex := 0; assignIndex < assigns.Count(); assignIndex++ {
			assign, present := assigns.At(assignIndex)
			if !present || !p.Flow().Executable().Contains(assign) {
				continue
			}
			count, countOK := assigns.WriteCount(assign)
			if !countOK {
				t.Fatalf("Assign(%v) WriteCount", assign)
			}
			wantWrites += count
		}
	}
	gotWrites := 0
	for index := 0; index < schema.StorageTransferCount(); index++ {
		transfer, ok := schema.StorageTransferAt(index)
		if !ok {
			t.Fatalf("StorageTransferAt(%d)", index)
		}
		ref, ok := transfer.Ref()
		if ok && ref.kind == storageTransferWrite {
			gotWrites++
		}
	}
	if wantWrites != width || gotWrites != wantWrites {
		t.Fatalf("wide assignment Write denominator = %d/%d, want %d", gotWrites, wantWrites, width)
	}
}

func storageTransferSchema(t testing.TB) (*Schema, *link.Link) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "storage_transfer.lua", Text: []byte("local n = 1\nlocal m = n\nn = m\ndo\n  return\nend\nlocal function dead(value)\n  local copy = value\n  return copy\nend\n")})
	if err != nil {
		t.Fatal(err)
	}
	return sealStorageTransferProgram(t, p)
}

func wideStorageTransferSchema(t testing.TB, width int) (*Schema, *link.Link) {
	t.Helper()
	var text strings.Builder
	text.WriteString("local ")
	for index := 0; index < width; index++ {
		if index != 0 {
			text.WriteString(", ")
		}
		text.WriteString(wideStorageVariable(index))
	}
	text.WriteString(" = ")
	for index := 0; index < width; index++ {
		if index != 0 {
			text.WriteString(", ")
		}
		text.WriteString("0")
	}
	text.WriteString("\n")
	for index := 0; index < width; index++ {
		if index != 0 {
			text.WriteString(", ")
		}
		text.WriteString(wideStorageVariable(index))
	}
	text.WriteString(" = ")
	for index := 0; index < width; index++ {
		if index != 0 {
			text.WriteString(", ")
		}
		text.WriteString(wideStorageVariable(index))
	}
	text.WriteString("\nreturn ")
	text.WriteString(wideStorageVariable(0))
	text.WriteString("\n")
	p, err := lower.Lower(lower.Source{Name: "storage_transfer_wide.lua", Text: []byte(text.String())})
	if err != nil {
		t.Fatal(err)
	}
	return sealStorageTransferProgram(t, p)
}

func wideStorageVariable(index int) string {
	return "v" + strconv.Itoa(index)
}

func sealStorageTransferProgram(t testing.TB, p *program.Program) (*Schema, *link.Link) {
	t.Helper()
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "storage_transfer", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, ok := Seal(linked, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema")
	}
	return schema, linked
}
