package artifact_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/lower"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
)

func TestArtifactRoundTripReplaysAuthoredProgram(t *testing.T) {
	contract := mustProfile(t)
	source := `
local function f(x: number): number
  if x > 0 then return x end
  return 0
end
local n = 2
while n > 0 and f(n) do n = n - 1 end
local root = {}
local alias = root
local m = require("module")
return root, alias, m, { f = f, nested = { f = f } }
`
	p := mustLower(t, "roundtrip.lua", source)
	metadata := artifact.Metadata{
		Provenance:   "sha256:source-revision",
		Dependencies: []artifact.Dependency{{Name: "module", ID: p.ContentID()}},
	}
	encoded, err := artifact.Encode(p, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	replayed, gotMetadata, err := artifact.Decode(encoded, contract)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ContentID() != p.ContentID() {
		t.Fatalf("Program ContentID = %v, want %v", replayed.ContentID(), p.ContentID())
	}
	entry, _ := p.Entry()
	replayedEntry, _ := replayed.Entry()
	if entry != replayedEntry {
		t.Fatalf("Entry = %v, want %v", replayedEntry, entry)
	}
	compareActivation := func(name string, before, after program.Term) {
		t.Helper()
		left, leftOK := p.Activation(before)
		right, rightOK := replayed.Activation(after)
		if left != right || leftOK != rightOK {
			t.Fatalf("%s Activation = %v/%v, want %v/%v", name, right, rightOK, left, leftOK)
		}
	}
	compareActivation("Entry", entry, replayedEntry)
	compareActivationDecisions := func(name string, beforeBody, afterBody program.Term) {
		t.Helper()
		beforeCount, beforeOK := p.ActivationDecisionCount(beforeBody)
		afterCount, afterOK := replayed.ActivationDecisionCount(afterBody)
		if beforeCount != afterCount || beforeOK != afterOK {
			t.Fatalf("%s activation decisions = %d/%v, want %d/%v", name, afterCount, afterOK, beforeCount, beforeOK)
		}
		for index := 0; index < beforeCount; index++ {
			left, leftOK := p.ActivationDecisionAt(beforeBody, index)
			right, rightOK := replayed.ActivationDecisionAt(afterBody, index)
			if left != right || leftOK != rightOK {
				t.Fatalf("%s activation decision[%d] = %v/%v, want %v/%v", name, index, right, rightOK, left, leftOK)
			}
		}
	}
	compareActivationDecisions("Entry", entry, replayedEntry)
	for index := 0; index < p.FunctionCount(); index++ {
		beforeFunction, _ := p.FunctionAt(index)
		afterFunction, _ := replayed.FunctionAt(index)
		_, beforeBody, _, beforeOK := p.Function(beforeFunction)
		_, afterBody, _, afterOK := replayed.Function(afterFunction)
		if !beforeOK || !afterOK {
			t.Fatalf("Function[%d] Body was not replayed", index)
		}
		compareActivation("Function", beforeFunction, afterFunction)
		compareActivation("Function Body", beforeBody, afterBody)
		compareActivationDecisions("Function", beforeBody, afterBody)
	}
	if span, ok := p.Span(entry); !ok {
		t.Fatal("original Entry has no source span")
	} else if replayedSpan, ok := replayed.Span(replayedEntry); !ok || replayedSpan != span {
		t.Fatalf("Entry span = %#v/%t, want %#v", replayedSpan, ok, span)
	}
	if replayed.ImplicitReadCount() != p.ImplicitReadCount() {
		t.Fatalf("implicit Read count = %d, want %d", replayed.ImplicitReadCount(), p.ImplicitReadCount())
	}
	for index := 0; index < p.ImplicitReadCount(); index++ {
		before, _ := p.ImplicitReadAt(index)
		after, _ := replayed.ImplicitReadAt(index)
		if before != after {
			t.Fatalf("implicit Read[%d] = %v, want %v", index, after, before)
		}
	}
	if replayed.ImportCount() != p.ImportCount() {
		t.Fatalf("Import count = %d, want %d", replayed.ImportCount(), p.ImportCount())
	}
	for index := 0; index < p.ImportCount(); index++ {
		before, _ := p.ImportAt(index)
		after, _ := replayed.ImportAt(index)
		beforeCall, beforeRequest, beforeModule, beforeAlias, beforeOK := p.Import(before)
		afterCall, afterRequest, afterModule, afterAlias, afterOK := replayed.Import(after)
		if !beforeOK || !afterOK || beforeCall != afterCall || beforeRequest != afterRequest || beforeModule != afterModule || beforeAlias != afterAlias {
			t.Fatalf("Import[%d] = %v/%v/%v/%v/%t, want %v/%v/%v/%v/%t", index, afterCall, afterRequest, afterModule, afterAlias, afterOK, beforeCall, beforeRequest, beforeModule, beforeAlias, beforeOK)
		}
	}
	if gotMetadata.Provenance != metadata.Provenance || len(gotMetadata.Dependencies) != 1 || gotMetadata.Dependencies[0] != metadata.Dependencies[0] {
		t.Fatalf("metadata = %#v, want %#v", gotMetadata, metadata)
	}
	reencoded, err := artifact.Encode(replayed, contract, gotMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("artifact roundtrip changed canonical bytes")
	}
	if replayed.ImportCount() != p.ImportCount() || replayed.EntryReturnCount() != p.EntryReturnCount() {
		t.Fatal("Seal-derived import/export projections were not reconstructed")
	}
	for returnIndex := 0; returnIndex < p.EntryReturnCount(); returnIndex++ {
		beforeReturn, _ := p.EntryReturnAt(returnIndex)
		afterReturn, _ := replayed.EntryReturnAt(returnIndex)
		_, beforeValues, beforeOK := p.Return(beforeReturn)
		_, afterValues, afterOK := replayed.Return(afterReturn)
		if !beforeOK || !afterOK {
			t.Fatalf("Entry export Return[%d] was not replayed", returnIndex)
		}
		width, _ := p.ValuesLen(beforeValues)
		if replayedWidth, ok := replayed.ValuesLen(afterValues); !ok || replayedWidth != width {
			t.Fatalf("Entry export Return[%d] width = %d/%v, want %d", returnIndex, replayedWidth, ok, width)
		}
		for ordinal := 0; ordinal < width; ordinal++ {
			beforeCell, beforeCellOK := p.EntryRootCell(beforeReturn, ordinal)
			afterCell, afterCellOK := replayed.EntryRootCell(afterReturn, ordinal)
			if afterCell != beforeCell || afterCellOK != beforeCellOK {
				t.Fatalf("Entry export Return[%d] Cell[%d] = %v/%v, want %v/%v", returnIndex, ordinal, afterCell, afterCellOK, beforeCell, beforeCellOK)
			}
		}
	}
	for index := 0; index < p.CallCount(); index++ {
		before, _ := p.CallAt(index)
		after, _ := replayed.CallAt(index)
		compareActivation("Call", before, after)
		if replayed.CallLive(after) != p.CallLive(before) {
			t.Fatalf("CallLive[%d] = %v, want %v", index, replayed.CallLive(after), p.CallLive(before))
		}
		left, leftOK := p.Mu(before)
		right, rightOK := replayed.Mu(after)
		if left != right || leftOK != rightOK {
			t.Fatalf("Mu Call[%d] = %v/%t, want %v/%t", index, right, rightOK, left, leftOK)
		}
		beforeCells, beforeCellsOK := p.CallContinuationCellCount(before)
		afterCells, afterCellsOK := replayed.CallContinuationCellCount(after)
		if beforeCells != afterCells || beforeCellsOK != afterCellsOK {
			t.Fatalf("continuation Cells Call[%d] = %d/%t, want %d/%t", index, afterCells, afterCellsOK, beforeCells, beforeCellsOK)
		}
		for ordinal := 0; ordinal < beforeCells; ordinal++ {
			left, leftOK := p.CallContinuationCellAt(before, ordinal)
			right, rightOK := replayed.CallContinuationCellAt(after, ordinal)
			if left != right || leftOK != rightOK {
				t.Fatalf("continuation Cell Call[%d][%d] = %v/%t, want %v/%t", index, ordinal, right, rightOK, left, leftOK)
			}
		}
		beforeValues, beforeValuesOK := p.CallContinuationValueCount(before)
		afterValues, afterValuesOK := replayed.CallContinuationValueCount(after)
		if beforeValues != afterValues || beforeValuesOK != afterValuesOK {
			t.Fatalf("continuation Values Call[%d] = %d/%t, want %d/%t", index, afterValues, afterValuesOK, beforeValues, beforeValuesOK)
		}
		for ordinal := 0; ordinal < beforeValues; ordinal++ {
			left, leftOK := p.CallContinuationValueAt(before, ordinal)
			right, rightOK := replayed.CallContinuationValueAt(after, ordinal)
			if left != right || leftOK != rightOK {
				t.Fatalf("continuation Value Call[%d][%d] = %v/%t, want %v/%t", index, ordinal, right, rightOK, left, leftOK)
			}
		}
	}
	for index := 0; index < p.OutcomeCount(); index++ {
		before, _ := p.OutcomeAt(index)
		after, _ := replayed.OutcomeAt(index)
		compareActivation("Outcome", before, after)
	}
	for index := 0; index < p.ReadCount(); index++ {
		before, _ := p.ReadAt(index)
		after, _ := replayed.ReadAt(index)
		left, leftOK := p.DirectFunction(before)
		right, rightOK := replayed.DirectFunction(after)
		if left != right || leftOK != rightOK {
			t.Fatalf("DirectFunction Read[%d] = %v/%v, want %v/%v", index, right, rightOK, left, leftOK)
		}
	}
	for index := 0; index < p.LoopCount(); index++ {
		before, _ := p.LoopAt(index)
		after, _ := replayed.LoopAt(index)
		beforeHead, beforeHeadOK := p.Mu(before)
		afterHead, afterHeadOK := replayed.Mu(after)
		if beforeHead != afterHead || beforeHeadOK != afterHeadOK {
			t.Fatalf("Loop[%d] Mu = %v/%v, want %v/%v", index, afterHead, afterHeadOK, beforeHead, beforeHeadOK)
		}
		if !beforeHeadOK || beforeHead != before {
			continue
		}
		beforeCount, beforeOK := p.MuDecisionCount(beforeHead)
		afterCount, afterOK := replayed.MuDecisionCount(afterHead)
		if beforeCount != afterCount || beforeOK != afterOK {
			t.Fatalf("Loop[%d] Mu decisions = %d/%v, want %d/%v", index, afterCount, afterOK, beforeCount, beforeOK)
		}
		for ordinal := 0; ordinal < beforeCount; ordinal++ {
			left, leftOK := p.MuDecisionAt(beforeHead, ordinal)
			right, rightOK := replayed.MuDecisionAt(afterHead, ordinal)
			if left != right || leftOK != rightOK {
				t.Fatalf("Loop[%d] Mu decision[%d] = %v/%v, want %v/%v", index, ordinal, right, rightOK, left, leftOK)
			}
		}
	}
}

func TestArtifactRejectsUnboundTargetAndNoncanonicalBytes(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "plain.lua", `local function f() f() end; f()`)
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := artifact.Decode(encoded, other); !errors.Is(err, artifact.ErrTargetMismatch) {
		t.Fatalf("wrong target = %v", err)
	}
	for _, corrupt := range [][]byte{encoded[:len(encoded)-1], append(append([]byte(nil), encoded...), 0), flip(encoded)} {
		if _, _, err := artifact.Decode(corrupt, contract); !errors.Is(err, artifact.ErrNoncanonical) {
			t.Fatalf("corrupt artifact = %v, want noncanonical error", err)
		}
	}
}

func TestArtifactCanonicalizesDependencyPermutationAndRejectsDuplicate(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "plain.lua", ``)
	first, err := artifact.Encode(p, contract, artifact.Metadata{Dependencies: []artifact.Dependency{{Name: "z", ID: p.ContentID()}, {Name: "a", ID: p.ContentID()}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := artifact.Encode(p, contract, artifact.Metadata{Dependencies: []artifact.Dependency{{Name: "a", ID: p.ContentID()}, {Name: "z", ID: p.ContentID()}}})
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("dependency permutation changed artifact bytes: %v", err)
	}
	_, err = artifact.Encode(p, contract, artifact.Metadata{Dependencies: []artifact.Dependency{{Name: "same", ID: p.ContentID()}, {Name: "same", ID: p.ContentID()}}})
	if err == nil {
		t.Fatal("duplicate dependency envelope encoded")
	}
}

func TestArtifactDeterminismUnderConcurrentConsumers(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "deterministic.lua", `local function f() f() end; return f`)
	metadata := artifact.Metadata{Provenance: "revision"}
	want, err := artifact.Encode(p, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	errs := make(chan error, 32)
	for worker := 0; worker < cap(errs); worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			encoded, err := artifact.Encode(p, contract, metadata)
			if err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(encoded, want) {
				errs <- errors.New("nondeterministic artifact bytes")
				return
			}
			replayed, got, err := artifact.Decode(encoded, contract)
			if err != nil {
				errs <- err
				return
			}
			if replayed.ContentID() != p.ContentID() {
				errs <- errors.New("concurrent replay changed Program identity")
				return
			}
			again, err := artifact.Encode(replayed, contract, got)
			if err != nil || !bytes.Equal(again, want) {
				errs <- errors.New("concurrent reencode changed artifact bytes")
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// TestEquationCacheWideFactorSchemaKeepsOneVersionPerProducer exercises the
// public persistence boundary with a wide declaration-ordered schema. One
// producer may have only one exact version: the alias is rejected rather than
// becoming an engine-local cache miss, while distinct producer order survives
// Encode/Decode unchanged.
func TestEquationCacheWideFactorSchemaKeepsOneVersionPerProducer(t *testing.T) {
	const width = 8192
	contract := mustProfile(t)
	p := mustLower(t, "wide-factor-schema.lua", "")
	bodies, ok := artifact.CanonicalEquationBodies(p)
	if !ok {
		t.Fatal("CanonicalEquationBodies")
	}
	factors := make([]artifact.SemanticKey, width)
	for index := range factors {
		factors[index] = wideArtifactSemanticKey(index)
	}
	cache := artifact.EquationCache{
		Program: p.ContentID(),
		Module:  wideArtifactContentID(width + 1),
		Engine:  wideArtifactSemanticKey(width + 2),
		Factors: factors,
		Bodies:  bodies,
	}
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "wide-factor-schema", Equations: &cache})
	if err != nil {
		t.Fatal(err)
	}
	_, metadata, err := artifact.Decode(encoded, contract)
	if err != nil || metadata.Equations == nil {
		t.Fatalf("Decode = %v, cache=%t", err, metadata.Equations != nil)
	}
	if len(metadata.Equations.Factors) != len(factors) {
		t.Fatalf("decoded factors=%d, want %d", len(metadata.Equations.Factors), len(factors))
	}
	for index := range factors {
		if metadata.Equations.Factors[index] != factors[index] {
			t.Fatalf("factor schema order changed at %d", index)
		}
	}
	aliased := cache
	aliased.Factors = append([]artifact.SemanticKey(nil), cache.Factors...)
	aliased.Factors[len(aliased.Factors)-1].ID = aliased.Factors[0].ID
	aliased.Factors[len(aliased.Factors)-1].Version++
	if _, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "wide-factor-schema", Equations: &aliased}); !errors.Is(err, artifact.ErrUnavailableProgram) {
		t.Fatalf("two versions of one factor producer encoded: %v", err)
	}
}

func TestEquationBoundaryReadSchemaRoundTripsExactCoordinates(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "equation-boundary.lua", "return 1")
	cache := equationBoundaryCache(t, p)
	factor := cache.Factors[0]
	cache.Boundary = []artifact.EquationBoundary{{
		Rule:       cache.Rules[0],
		Output:     factor,
		InputArity: 2,
		Activation: equationBoundaryActivation(t, p),
		At:         equationBoundaryAt(t, p),
		Reads: []artifact.EquationRead{
			{Position: 0, Factor: factor},
			{Position: 1, Factor: factor},
		},
	}}
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "exact-equation-reads", Equations: &cache})
	if err != nil {
		t.Fatal(err)
	}
	_, metadata, err := artifact.Decode(encoded, contract)
	if err != nil || metadata.Equations == nil {
		t.Fatalf("Decode = %v, cache=%t", err, metadata.Equations != nil)
	}
	got := metadata.Equations.Boundary
	if len(got) != 1 || got[0].InputArity != 2 || len(got[0].Reads) != 2 {
		t.Fatalf("boundary schema = %#v", got)
	}
	for position, read := range got[0].Reads {
		if read.Position != position || read.Factor != factor {
			t.Fatalf("read[%d] = %#v, want position %d factor %#v", position, read, position, factor)
		}
	}
}

func TestEquationBoundaryReadSchemaRejectsInvalidCoordinates(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "equation-boundary-invalid.lua", "return 1")
	base := equationBoundaryCache(t, p)
	factor := base.Factors[0]
	validBoundary := func() artifact.EquationBoundary {
		return artifact.EquationBoundary{
			Rule:       base.Rules[0],
			Output:     factor,
			InputArity: 2,
			Activation: equationBoundaryActivation(t, p),
			At:         equationBoundaryAt(t, p),
			Reads:      []artifact.EquationRead{{Position: 0, Factor: factor}},
		}
	}
	for _, row := range []struct {
		name   string
		adjust func(*artifact.EquationBoundary)
	}{
		{name: "negative arity", adjust: func(boundary *artifact.EquationBoundary) { boundary.InputArity = -1 }},
		{name: "zero arity", adjust: func(boundary *artifact.EquationBoundary) { boundary.InputArity = 0 }},
		{name: "negative position", adjust: func(boundary *artifact.EquationBoundary) { boundary.Reads[0].Position = -1 }},
		{name: "position outside arity", adjust: func(boundary *artifact.EquationBoundary) { boundary.Reads[0].Position = 2 }},
		{name: "noncanonical permutation", adjust: func(boundary *artifact.EquationBoundary) {
			boundary.Reads = []artifact.EquationRead{{Position: 1, Factor: factor}, {Position: 0, Factor: factor}}
		}},
		{name: "duplicate exact pair", adjust: func(boundary *artifact.EquationBoundary) {
			boundary.Reads = []artifact.EquationRead{{Position: 0, Factor: factor}, {Position: 0, Factor: factor}}
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			cache := base
			cache.Boundary = []artifact.EquationBoundary{validBoundary()}
			row.adjust(&cache.Boundary[0])
			if _, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "invalid-equation-read", Equations: &cache}); !errors.Is(err, artifact.ErrUnavailableProgram) {
				t.Fatalf("Encode error = %v, want unavailable Program", err)
			}
		})
	}
}

func TestEquationBoundaryReadSchemaAcceptsEmptyReadsAndChangesCanonicalBytes(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "equation-boundary-bytes.lua", "return 1")
	base := equationBoundaryCache(t, p)
	factor := base.Factors[0]
	boundary := artifact.EquationBoundary{
		Rule:       base.Rules[0],
		Output:     factor,
		InputArity: 2,
		Activation: equationBoundaryActivation(t, p),
		At:         equationBoundaryAt(t, p),
		Reads:      []artifact.EquationRead{{Position: 0, Factor: factor}},
	}
	encode := func(boundary artifact.EquationBoundary) []byte {
		t.Helper()
		cache := base
		cache.Boundary = []artifact.EquationBoundary{boundary}
		data, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "equation-read-bytes", Equations: &cache})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	withRead := encode(boundary)
	changedPosition := boundary
	changedPosition.Reads = []artifact.EquationRead{{Position: 1, Factor: factor}}
	if bytes.Equal(withRead, encode(changedPosition)) {
		t.Fatal("changing a read position did not change canonical bytes")
	}
	changedArity := boundary
	changedArity.InputArity = 3
	if bytes.Equal(withRead, encode(changedArity)) {
		t.Fatal("changing input arity did not change canonical bytes")
	}
	empty := boundary
	empty.InputArity = 1
	empty.Reads = nil
	if data := encode(empty); len(data) == 0 {
		t.Fatal("zero-read boundary did not encode")
	}
}

func equationBoundaryCache(t *testing.T, p *program.Program) artifact.EquationCache {
	t.Helper()
	bodies, ok := artifact.CanonicalEquationBodies(p)
	if !ok {
		t.Fatal("CanonicalEquationBodies")
	}
	return artifact.EquationCache{
		Program: p.ContentID(),
		Module:  wideArtifactContentID(41),
		Engine:  wideArtifactSemanticKey(42),
		Factors: []artifact.SemanticKey{wideArtifactSemanticKey(43)},
		Rules:   []artifact.SemanticKey{wideArtifactSemanticKey(44)},
		Bodies:  bodies,
	}
}

func equationBoundaryAt(t *testing.T, p *program.Program) program.Term {
	t.Helper()
	body, ok := p.BodyAt(0)
	if !ok {
		t.Fatal("BodyAt(0)")
	}
	return body
}

func equationBoundaryActivation(t *testing.T, p *program.Program) program.Term {
	t.Helper()
	activation, ok := p.Activation(equationBoundaryAt(t, p))
	if !ok {
		t.Fatal("Activation")
	}
	return activation
}

func wideArtifactSemanticKey(index int) artifact.SemanticKey {
	return artifact.SemanticKey{ID: wideArtifactContentID(index), Version: uint64(index%7 + 1)}
}

func wideArtifactContentID(index int) (id program.ContentID) {
	id[0] = byte(index>>8) + 1
	id[1] = byte(index)
	return id
}

func mustProfile(t *testing.T) *target.Contract {
	t.Helper()
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
func mustLower(t *testing.T, name, source string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: name, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func flip(data []byte) []byte {
	copy := append([]byte(nil), data...)
	copy[len(copy)/2] ^= 1
	return copy
}
