// publication_column_law_test.go states the write-capability laws: a published
// column is filled through the one capability the engine minted for it and
// through nothing else, and the published value carries none.

package engine

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

const (
	lawColumnOutput schema.Key = "law/column"
	lawPeerOutput   schema.Key = "law/peer"
	lawColumnWriter schema.Key = "law/writer"
	lawOtherWriter  schema.Key = "law/other-writer"
)

var (
	lawColumnTable      = identity.ContentID{0x21, 0x01}
	lawColumnStore      = identity.StoreID(11)
	lawColumnGeneration = identity.Generation(1)
	lawColumnAxis       = snapshot.Axis[uint64, uint64]{SchemaID: lawColumnTable, Slot: 0}
)

// lawColumnAdmissions is the sealed table's issuance request for the two law
// columns, in slot order.
func lawColumnAdmissions() []ColumnAdmission {
	return []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: lawColumnTable, Output: lawPeerOutput, Writer: lawColumnWriter, Slot: 1},
	}
}

// lawOpenColumnBinding returns an open binding whose factor and query cells are
// complete, so the only thing left to state about it is its column admissions.
func lawOpenColumnBinding(t testing.TB) *SchemaBinding {
	t.Helper()
	_, factor, query := exactQuerySchemaFixture(t)
	binding := NewSchemaBinding(factor.Schema())
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) {
		t.Fatal("law column binding")
	}
	return binding
}

// lawSealedColumnBinding seals one binding that admits the two law columns.
func lawSealedColumnBinding(t testing.TB) *SchemaBinding {
	t.Helper()
	binding := lawOpenColumnBinding(t)
	if !AdmitColumns(binding, lawColumnAdmissions()) || !binding.Seal() {
		t.Fatal("law column admission")
	}
	return binding
}

// TestColumnWriteWithoutAMintedCapabilityIsRefused states the forgery law. A
// ColumnWrite holds one unexported grant, so the only value a package outside
// the engine can produce is the zero one, and the zero one unlocks no column.
// The same builder the forged capability is refused on is filled by the minted
// one, so what the refusal states is the capability and not the builder.
func TestColumnWriteWithoutAMintedCapabilityIsRefused(t *testing.T) {
	binding := lawSealedColumnBinding(t)
	builder := snapshot.NewBuilder(lawColumnTable, lawColumnStore, lawColumnGeneration)
	var forged ColumnWrite[uint64, uint64]
	if forged.Available() {
		t.Fatal("the zero write capability reports itself available")
	}
	if err := PublishColumn(forged, &builder, snapshot.Content[uint64, uint64]{Rows: map[uint64]uint64{1: 10}}); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability sealed a column: %v", err)
	}
	if err := PublishRow(forged, &builder, 1, 10); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability published a row: %v", err)
	}
	if err := WithdrawRow(forged, &builder, 1); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability withdrew a row: %v", err)
	}
	if _, err := PublishQueryColumn(forged, &builder, lawColumnTable, snapshot.Content[uint64, uint64]{Rows: map[uint64]uint64{1: 10}}); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability declared a query column: %v", err)
	}
	write, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter)
	if !minted || !write.Available() {
		t.Fatal("the sealed admission mints no write capability")
	}
	if err := PublishColumn(write, &builder, snapshot.Content[uint64, uint64]{Rows: map[uint64]uint64{1: 10}}); err != nil {
		t.Fatalf("the minted capability sealed no column: %v", err)
	}
	if err := PublishRow(write, &builder, 2, 20); err != nil {
		t.Fatalf("the minted capability published no row: %v", err)
	}
	if err := WithdrawRow(write, &builder, 2); err != nil {
		t.Fatalf("the minted capability withdrew no row: %v", err)
	}
}

// TestOneColumnMintsOneWriteCapability states the runtime end of the seal's
// one-writer law. The table admits one writer per column, so the engine mints
// one capability per column: a second mint of the same column is refused
// whatever key and value types it claims, and a mint naming a writer the table
// did not admit is refused as well.
func TestOneColumnMintsOneWriteCapability(t *testing.T) {
	binding := lawSealedColumnBinding(t)
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); !minted {
		t.Fatal("the admitted column mints no write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); minted {
		t.Fatal("one column minted a second write capability")
	}
	if _, minted := MintColumnWrite[string, uint64](binding, lawColumnOutput, lawColumnWriter); minted {
		t.Fatal("a second claim over one column minted another write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawPeerOutput, lawOtherWriter); minted {
		t.Fatal("a writer the table never admitted minted a write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, "law/undeclared", lawColumnWriter); minted {
		t.Fatal("a column the table never declared minted a write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawPeerOutput, lawColumnWriter); !minted {
		t.Fatal("the second admitted column mints no write capability of its own")
	}
}

// TestWriteCapabilityRequiresASealedAdmission states that the admitted set and
// the minted capability are one law at two ends. A binding that has not sealed
// its admissions mints nothing, and an admission stated after the seal reaches
// no column.
func TestWriteCapabilityRequiresASealedAdmission(t *testing.T) {
	binding := lawOpenColumnBinding(t)
	if !AdmitColumns(binding, lawColumnAdmissions()) {
		t.Fatal("the open binding admits no columns")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); minted {
		t.Fatal("an unsealed binding minted a write capability")
	}
	if !binding.Seal() {
		t.Fatal("the admitted binding does not seal")
	}
	if AdmitColumns(binding, lawColumnAdmissions()) {
		t.Fatal("a sealed binding admitted a column")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); !minted {
		t.Fatal("the sealed admission mints no write capability")
	}
}

// TestAdmittedColumnSetIsStatedOnceAndWithoutCollision states the admission's
// own law. The set names each column once and each slot once, and it is stated
// once: a table that named one column twice, or two columns one slot, would
// leave the engine holding two writers for one column.
func TestAdmittedColumnSetIsStatedOnceAndWithoutCollision(t *testing.T) {
	restated := lawOpenColumnBinding(t)
	if !AdmitColumns(restated, lawColumnAdmissions()) {
		t.Fatal("the open binding admits no columns")
	}
	if AdmitColumns(restated, lawColumnAdmissions()) {
		t.Fatal("the admitted column set was stated twice")
	}
	if restated.Seal() || !restated.Poisoned() {
		t.Fatal("a binding whose admissions were restated sealed anyway")
	}

	duplicateOutput := lawOpenColumnBinding(t)
	if AdmitColumns(duplicateOutput, []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawOtherWriter, Slot: 1},
	}) {
		t.Fatal("one column was admitted for two writers")
	}

	duplicateSlot := lawOpenColumnBinding(t)
	if AdmitColumns(duplicateSlot, []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: lawColumnTable, Output: lawPeerOutput, Writer: lawColumnWriter, Slot: 0},
	}) {
		t.Fatal("two columns were admitted into one slot")
	}

	foreignTable := lawOpenColumnBinding(t)
	if AdmitColumns(foreignTable, []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: identity.ContentID{0x99}, Output: lawPeerOutput, Writer: lawColumnWriter, Slot: 1},
	}) {
		t.Fatal("columns of two tables were admitted into one publication")
	}

	incomplete := lawOpenColumnBinding(t)
	if AdmitColumns(incomplete, []ColumnAdmission{{Schema: lawColumnTable, Output: lawColumnOutput, Slot: 0}}) {
		t.Fatal("a column with no admitted writer was admitted")
	}
}

// TestPublishedSnapshotCarriesNoWriteCapability states that the capability
// lives with the engine and never with the value. A reader recovers a
// published row from an address alone, and the capability that wrote a sealed
// publication cannot reach back into it: the next generation it writes is a new
// value and the sealed one still answers exactly what it was sealed with.
func TestPublishedSnapshotCarriesNoWriteCapability(t *testing.T) {
	binding := lawSealedColumnBinding(t)
	write, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter)
	peer, peerMinted := MintColumnWrite[uint64, uint64](binding, lawPeerOutput, lawColumnWriter)
	if !minted || !peerMinted {
		t.Fatal("the sealed admission mints no write capability")
	}
	builder := snapshot.NewBuilder(lawColumnTable, lawColumnStore, lawColumnGeneration)
	if err := PublishColumn(write, &builder, snapshot.Content[uint64, uint64]{Rows: map[uint64]uint64{1: 10}}); err != nil {
		t.Fatalf("seal law column: %v", err)
	}
	if err := PublishColumn(peer, &builder, snapshot.Content[uint64, uint64]{}); err != nil {
		t.Fatalf("seal law peer column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal law publication: %v", err)
	}
	if value, status := snapshot.Read(&sealed, lawColumnAxis, 1); status != snapshot.ReadHit || value != 10 {
		t.Fatalf("the published row reads %d as %s from its address alone", value, status)
	}

	next := snapshot.NewDelta(sealed, lawColumnGeneration+1)
	if err := PublishRow(write, &next, 1, 99); err != nil {
		t.Fatalf("publish the next generation: %v", err)
	}
	following, err := next.Seal()
	if err != nil {
		t.Fatalf("seal the next generation: %v", err)
	}
	if value, status := snapshot.Read(&sealed, lawColumnAxis, 1); status != snapshot.ReadHit || value != 10 {
		t.Fatalf("the sealed publication moved to %d (%s) under a held capability", value, status)
	}
	if value, status := snapshot.Read(&following, lawColumnAxis, 1); status != snapshot.ReadHit || value != 99 {
		t.Fatalf("the following publication reads %d as %s", value, status)
	}
}

func TestColumnBindingMintsTheSolvedPublicationWrites(t *testing.T) {
	schema := identity.ContentID{0x31, 0x02}
	binding := NewColumnBinding()
	if binding == nil || binding.Seal() {
		t.Fatal("a column binding sealed with no admissions")
	}
	binding = NewColumnBinding()
	if !AdmitColumns(binding, []ColumnAdmission{
		{Schema: schema, Output: solvedQueryOutput, Writer: solvedColumnWriter, Slot: 0},
		{Schema: schema, Output: solvedObservationOutput, Writer: solvedColumnWriter, Slot: 1},
		{Schema: schema, Output: solvedPointOutput, Writer: solvedColumnWriter, Slot: solvedPointSlot},
	}) || !binding.Seal() {
		t.Fatal("solved column admissions")
	}
	if _, minted := MintColumnWrite[identity.ContentID, Answer](binding, solvedQueryOutput, solvedColumnWriter); !minted {
		t.Fatal("query column mints no write")
	}
	if _, minted := MintColumnWrite[identity.ContentID, Answer](binding, solvedObservationOutput, solvedColumnWriter); !minted {
		t.Fatal("observation column mints no write")
	}
	if _, minted := MintColumnWrite[composition.Key, carrier.PointState](binding, solvedPointOutput, solvedColumnWriter); !minted {
		t.Fatal("point-state column mints no write")
	}
	var forged ColumnWrite[identity.ContentID, Answer]
	builder := snapshot.NewBuilder(schema, lawColumnStore, lawColumnGeneration)
	if _, err := PublishQueryColumn(forged, &builder, schema, snapshot.Content[identity.ContentID, Answer]{}); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability declared a query column: %v", err)
	}
}

func TestSnapshotWriteVerbsRequireColumnWrite(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	verbs := map[string]struct{}{"PutColumn": {}, "SetRow": {}, "RemoveRow": {}, "DeclareQuery": {}}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if parseErr != nil {
			t.Fatalf("%s: %v", name, parseErr)
		}
		for _, decl := range file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Body == nil {
				continue
			}
			writes := false
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}
				callee := call.Fun
				switch indexed := callee.(type) {
				case *ast.IndexExpr:
					callee = indexed.X
				case *ast.IndexListExpr:
					callee = indexed.X
				}
				selector, isSelector := callee.(*ast.SelectorExpr)
				if !isSelector {
					return true
				}
				base, isIdent := selector.X.(*ast.Ident)
				if !isIdent || base.Name != "snapshot" {
					return true
				}
				if _, isVerb := verbs[selector.Sel.Name]; isVerb {
					writes = true
				}
				return true
			})
			if !writes {
				continue
			}
			if !funcHasColumnWriteParam(fn) {
				t.Errorf("%s: %s reaches a snapshot write verb without a ColumnWrite parameter", name, fn.Name.Name)
			}
		}
	}
}

func funcHasColumnWriteParam(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		if typeNamesColumnWrite(field.Type) {
			return true
		}
	}
	return false
}

func typeNamesColumnWrite(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.IndexExpr:
		return typeNamesColumnWrite(typed.X)
	case *ast.IndexListExpr:
		return typeNamesColumnWrite(typed.X)
	case *ast.Ident:
		return typed.Name == "ColumnWrite"
	}
	return false
}

var (
	lawQueryFamily      = identity.ContentID{0x26, 0x06}
	lawQueryDenominator = identity.ContentID{0x27, 0x07}
)

func lawMintedQueryWrite(t testing.TB) ColumnWrite[uint64, uint64] {
	t.Helper()
	binding := NewColumnBinding()
	if !AdmitColumns(binding, []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
	}) || !binding.Seal() {
		t.Fatal("query column admission")
	}
	write, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter)
	if !minted || !write.Available() {
		t.Fatal("query column mints no write")
	}
	return write
}

func lawPublishQuery(t testing.TB, write ColumnWrite[uint64, uint64], generation identity.Generation, members []uint64, rows map[uint64]uint64) snapshot.Snapshot {
	t.Helper()
	builder := snapshot.NewBuilder(lawColumnTable, lawColumnStore, generation)
	if _, err := PublishQueryColumn(write, &builder, lawQueryFamily, snapshot.Content[uint64, uint64]{
		Rows: rows, Denominator: lawQueryDenominator, Members: members,
	}); err != nil {
		t.Fatalf("declare query column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal query publication: %v", err)
	}
	return sealed
}

func TestPublishedQueryColumnAnswersMembersAndProvesEveryOtherAbsence(t *testing.T) {
	write := lawMintedQueryWrite(t)
	published := lawPublishQuery(t, write, lawColumnGeneration, []uint64{1, 2, 3, 4}, map[uint64]uint64{1: 17})
	plan, opened := snapshot.OpenQuery[uint64, uint64](&published, lawQueryFamily)
	if !opened {
		t.Fatal("the published family opens no plan")
	}
	if answer, status := snapshot.Query(&published, plan, 1); status != snapshot.ReadHit || answer != 17 {
		t.Fatalf("the answered member reads %d as %s, not 17 as a hit", answer, status)
	}
	if answer, status := snapshot.Query(&published, plan, 2); status != snapshot.ReadProvenAbsent {
		t.Fatalf("a covered unanswered member reads %d as %s, not a proven absence", answer, status)
	}
	if _, status := snapshot.Query(&published, plan, 99); status != snapshot.ReadMiss {
		t.Fatalf("a subject outside the published universe reads as %s, not ignorance", status)
	}
}

func TestWithdrawnCoveredMemberReadsProvenAbsentAndLeavesThePriorPublication(t *testing.T) {
	write := lawMintedQueryWrite(t)
	published := lawPublishQuery(t, write, lawColumnGeneration, []uint64{1, 2}, map[uint64]uint64{1: 17, 2: 4})
	next := snapshot.NewDelta(published, lawColumnGeneration+1)
	if err := WithdrawRow(write, &next, 1); err != nil {
		t.Fatalf("withdraw covered member: %v", err)
	}
	following, err := next.Seal()
	if err != nil {
		t.Fatalf("seal the withdrawal: %v", err)
	}
	plan, opened := snapshot.OpenQuery[uint64, uint64](&following, lawQueryFamily)
	if !opened {
		t.Fatal("the following generation opens no plan")
	}
	if answer, status := snapshot.Query(&following, plan, 1); status != snapshot.ReadProvenAbsent {
		t.Fatalf("the withdrawn member reads %d as %s, not a proven absence", answer, status)
	}
	if answer, status := snapshot.Query(&following, plan, 2); status != snapshot.ReadHit || answer != 4 {
		t.Fatalf("an untouched member reads %d as %s, not 4 as a hit", answer, status)
	}
	prior, priorOpened := snapshot.OpenQuery[uint64, uint64](&published, lawQueryFamily)
	if !priorOpened {
		t.Fatal("the prior generation opens no plan")
	}
	if answer, status := snapshot.Query(&published, prior, 1); status != snapshot.ReadHit || answer != 17 {
		t.Fatalf("the prior generation moved to %d (%s) under the withdrawal", answer, status)
	}
}

func TestUnsealedDeltaLeavesTheBasePublicationUntouched(t *testing.T) {
	write := lawMintedQueryWrite(t)
	published := lawPublishQuery(t, write, lawColumnGeneration, []uint64{1, 2}, map[uint64]uint64{1: 10, 2: 4})
	abandoned := snapshot.NewDelta(published, lawColumnGeneration+1)
	if err := PublishRow(write, &abandoned, 1, 99); err != nil {
		t.Fatalf("edit the abandoned delta: %v", err)
	}
	plan, opened := snapshot.OpenQuery[uint64, uint64](&published, lawQueryFamily)
	if !opened {
		t.Fatal("the publication opens no plan after the abandoned delta")
	}
	if answer, status := snapshot.Query(&published, plan, 1); status != snapshot.ReadHit || answer != 10 {
		t.Fatalf("the publication answers %d as %s after an unsealed delta", answer, status)
	}
}

func TestDeltaCostFollowsTheChangeSetAndNotTheColumn(t *testing.T) {
	small := lawDeltaAllocations(t, 8)
	large := lawDeltaAllocations(t, 512)
	if large > small*2 {
		t.Fatalf("a one-subject delta allocates %.0f over 8 subjects and %.0f over 512, so its cost follows the column", small, large)
	}
}

func lawDeltaAllocations(t testing.TB, width uint64) float64 {
	t.Helper()
	members := make([]uint64, 0, width)
	rows := make(map[uint64]uint64, width)
	for key := uint64(1); key <= width; key++ {
		members = append(members, key)
		rows[key] = key
	}
	write := lawMintedQueryWrite(t)
	published := lawPublishQuery(t, write, lawColumnGeneration, members, rows)
	return testing.AllocsPerRun(64, func() {
		next := snapshot.NewDelta(published, lawColumnGeneration+1)
		if err := PublishRow(write, &next, 1, 1); err != nil {
			t.Fatalf("publish a one-subject delta: %v", err)
		}
		if _, err := next.Seal(); err != nil {
			t.Fatalf("seal a one-subject delta: %v", err)
		}
	})
}
