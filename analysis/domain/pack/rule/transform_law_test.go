package rule

import (
	"context"
	"crypto/sha256"
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/analysis/domain/pack/source"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestScalarizationRunsThroughOneSourceAssembly exercises the actual engine
// path: an authored Pack source seeds one call occurrence, then the Pack
// scalarization Rule writes a distinct Pack root. The test observes only the
// output factor, so a direct Go projection cannot masquerade as a solved fact.
func TestScalarizationRunsThroughOneSourceAssembly(t *testing.T) {
	schema := transformSchema(t)
	var input, output packdomain.Root
	for index := 0; index < schema.RootCount(); index++ {
		root, ok := schema.RootAt(index)
		if !ok {
			continue
		}
		if _, sourceOK := schema.Source(root); !sourceOK {
			continue
		}
		if input == (packdomain.Root{}) {
			input = root
		} else if output == (packdomain.Root{}) {
			output = root
			break
		}
	}
	if input == (packdomain.Root{}) || output == (packdomain.Root{}) {
		t.Fatal("fixture did not expose two Pack source roots")
	}
	inputSource, ok := schema.Source(input)
	if !ok {
		t.Fatal("input source")
	}
	composition := engine.NewComposition()
	owner, ok := packowner.Declare(composition, transformKey(1), schema)
	if !ok {
		t.Fatal("Pack owner")
	}
	seed, ok := packsource.Declare(composition, transformKey(2), transformKey(3), transformKey(4), owner)
	if !ok {
		t.Fatal("source Rule")
	}
	offset, ok := schema.TableIndex(0)
	if !ok {
		t.Fatal("zero TableIndex")
	}
	scalarization, ok := NewScalarization(schema, input, output, offset)
	if !ok {
		t.Fatal("scalarization operand")
	}
	adjustment, ok := DeclareScalarization(composition, transformKey(5), transformKey(6), transformKey(7), owner)
	if !ok {
		t.Fatal("scalarization Rule")
	}
	var observed engine.QueryRead[engine.OrderedCells[packdomain.Value]]
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transformKey(8),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, observed)
				value, present, valueOK := cells.At(0)
				return rows == 1 && cellsOK && valueOK && present && !value.IsBottom()
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: transformKey(9), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		observed, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !ok || query == nil {
		t.Fatal("query")
	}
	if !composition.Seal() {
		t.Fatal("composition seal")
	}
	seedInstance, ok := seed.Instance(inputSource)
	if !ok {
		t.Fatal("source instance")
	}
	adjustmentInstance, ok := adjustment.Instance(scalarization)
	if !ok {
		t.Fatal("scalarization instance")
	}
	result := testlaw.RunLinear(context.Background(), testlaw.LinearFixture[packdomain.Value, packdomain.Source, packdomain.Value, Scalarization, bool]{
		Composition: composition, Source: seedInstance, Steps: []*engine.RuleInstance[packdomain.Value, Scalarization]{adjustmentInstance}, Query: query,
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			ref, refOK := owner.Locate(output)
			return refOK && engine.InstanceQueryRead(binding, observed, ref)
		},
		SourceSite: transformKey(10), SourceOccurrence: transformKey(11), StepSites: []engine.SemanticKey{transformKey(12)}, StepOccurrences: []engine.SemanticKey{transformKey(13)}, BoundarySemantics: []engine.SemanticKey{transformKey(14)},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("scalarization Solver result: status=%v available=%v value=%v", result.Status, result.ValueAvailable, result.Value)
	}
}

func TestScalarizationRejectsForeignTableIndexWithSameValue(t *testing.T) {
	local := transformSchema(t)
	foreign := transformSchema(t)
	var input, output packdomain.Root
	for index := 0; index < local.RootCount(); index++ {
		root, rootOK := local.RootAt(index)
		if !rootOK {
			continue
		}
		if _, sourceOK := local.Source(root); !sourceOK {
			continue
		}
		if input == (packdomain.Root{}) {
			input = root
		} else {
			output = root
			break
		}
	}
	foreignIndex, foreignOK := foreign.TableIndex(0)
	if input == (packdomain.Root{}) || output == (packdomain.Root{}) || !foreignOK {
		t.Fatal("foreign TableIndex fixture")
	}
	if _, ok := NewScalarization(local, input, output, foreignIndex); ok {
		t.Fatal("foreign TableIndex crossed Scalarization owner fence")
	}
}

func transformKey(value byte) engine.SemanticKey {
	digest := sha256.Sum256([]byte{value, 0x50, 0x41, 0x43, 0x4b})
	key, _ := engine.NewSemanticKey(digest, 1)
	return key
}

// bodyLawSeed is a test-only trusted theorem operand.  It carries no domain
// coordinate or raw Program term: the value is captured by the declaration,
// while the operand commits only to the reviewed theorem identity.
type bodyLawSeed struct{ id [32]byte }

func bodyLawSeedContent(seed bodyLawSeed) (bodyLawSeed, [32]byte, bool) {
	return seed, seed.id, seed.id != [32]byte{}
}

type bottomSummarySeed struct{ id [32]byte }

func bottomSummarySeedContent(seed bottomSummarySeed) (bottomSummarySeed, [32]byte, bool) {
	return seed, seed.id, seed.id != [32]byte{}
}

type bottomSummarySeedDeclaration struct {
	rule  *engine.Rule[packdomain.Value, bottomSummarySeed]
	write engine.Write[packdomain.Value]
	owner *packowner.Owner
}

func (declaration *bottomSummarySeedDeclaration) Instance(root packdomain.Root, seed bottomSummarySeed) (*engine.RuleInstance[packdomain.Value, bottomSummarySeed], bool) {
	if declaration == nil || declaration.rule == nil || declaration.owner == nil {
		return nil, false
	}
	ref, refOK := declaration.owner.Locate(root)
	if !refOK {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, seed, func(binding *engine.RuleBinding[packdomain.Value, bottomSummarySeed]) bool {
		return engine.InstanceWrite(binding, declaration.write, ref)
	})
}

func declareBottomSummarySeed(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner) (*bottomSummarySeedDeclaration, bool) {
	if composition == nil || owner == nil {
		return nil, false
	}
	var write engine.Write[packdomain.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, bottomSummarySeed]{
		Semantic: semantic, OperandFamily: family, OperandContent: bottomSummarySeedContent,
		Output: owner.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[packdomain.Value, bottomSummarySeed](evidence),
		Transfer: func(access engine.Access[packdomain.Value, bottomSummarySeed]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.NoCandidate(access, row) })
		},
	}, func(rule *engine.Rule[packdomain.Value, bottomSummarySeed]) bool {
		var writeOK bool
		write, writeOK = engine.WriteTo(rule, owner.ExactWrite())
		return writeOK
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &bottomSummarySeedDeclaration{rule: declared, write: write, owner: owner}, true
}

func transformSchema(t testing.TB) *packdomain.Schema {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "pack_transform_law.lua", Text: []byte(`
local function many(...) return ... end
local object = {}
object:send(1, many(2))
`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"selected"}}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: target.ValuesVariable, Var: 0}, ValuesVars: 1,
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_transform_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := packdomain.Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}
	return schema
}

func outcomeTransferLawSchema(t testing.TB) *packdomain.Schema {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "pack_outcome_bottom_law.lua", Text: []byte(`
local function branch(flag)
  if flag then
    return 1
  elseif not flag then
    return 2
  end
end
branch(true)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_outcome_bottom_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := packdomain.Seal(linked, statics)
	if !ok {
		t.Fatal("Pack outcome schema")
	}
	return schema
}

func transformBindSchema(t testing.TB) *packdomain.Schema {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "pack_bind_transform_law.lua", Text: []byte(`
local function many(...) return ... end
local a, b = 1
local c, d = many(2)
`)})
	if err != nil {
		t.Fatal(err)
	}
	binding := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"selected"}}
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesVariable, Var: 0}, ValuesVars: 1,
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_bind_transform_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := packdomain.Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}
	return schema
}

func transformBodySchema(t testing.TB) *packdomain.Schema {
	return transformBodySchemaMode(t, true)
}

func transformBodySchemaMode(t testing.TB, tail bool) *packdomain.Schema {
	t.Helper()
	callLine := "receiver:callee(...)"
	if tail {
		callLine = "return receiver:callee(...)"
	}
	program, err := lower.Lower(lower.Source{Name: "pack_body_transform_law.lua", Text: []byte(`
local receiver = {}
function receiver:callee(a, b, ...)
  return a, b, ...
end
local function caller(...)
` + callLine + `
end
caller(1, 2)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_body_transform_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := packdomain.Seal(linked, statics)
	if !ok {
		t.Fatal("Pack schema")
	}
	return schema
}

func bindDescriptorWithShape(linked *link.Link, schema *packdomain.Schema, width int, open bool) (packdomain.Bind, bool) {
	if linked == nil || schema == nil {
		return packdomain.Bind{}, false
	}
	mounts := schema.Link().Project().Mounts()
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		program, programOK := mounts.Program(shard)
		if !shardOK || !programOK || program == nil {
			continue
		}
		binds := program.Flow().Authored().Storage().Binds()
		for bindIndex := 0; bindIndex < binds.Count(); bindIndex++ {
			term, termOK := binds.At(bindIndex)
			if !termOK {
				continue
			}
			bind, bindOK := schema.Bind(shard, term)
			if !bindOK || bind.CellCount() != width {
				continue
			}
			inputRoot, inputRootOK := bind.InputRoot()
			inputSource, inputSourceOK := schema.Source(inputRoot)
			if !inputRootOK || !inputSourceOK {
				continue
			}
			item, itemOK := inputSource.At(0)
			if !itemOK {
				continue
			}
			_, _, hasTail := item.Tail()
			if hasTail == open {
				return bind, true
			}
		}
	}
	return packdomain.Bind{}, false
}

func sourceDescriptorTerm(schema *packdomain.Schema, root packdomain.Root) (packdomain.Term, bool) {
	source, sourceOK := schema.Source(root)
	item, itemOK := source.At(0)
	builder, builderOK := schema.Builder(root)
	if !sourceOK || !itemOK || !builderOK {
		return packdomain.Term{}, false
	}
	fixed := make([]packdomain.Scalar, item.FixedCount())
	for index := range fixed {
		endpoint, endpointOK := item.FixedAt(index)
		if !endpointOK {
			return packdomain.Term{}, false
		}
		fixed[index], endpointOK = builder.Endpoint(endpoint)
		if !endpointOK {
			return packdomain.Term{}, false
		}
	}
	tail, offset, open := item.Tail()
	if !open {
		return builder.Closed(fixed...)
	}
	free, freeOK := builder.FreeTail(tail)
	if !freeOK {
		return packdomain.Term{}, false
	}
	rest, restOK := builder.Tail(free, offset)
	if !restOK {
		return packdomain.Term{}, false
	}
	return builder.Open(fixed, rest, nil)
}

func runBindLaw(t *testing.T, schema *packdomain.Schema, bind packdomain.Bind, wantOpen bool) {
	t.Helper()
	input, inputOK := bind.InputRoot()
	output, outputOK := bind.Root()
	inputSource, sourceOK := schema.Source(input)
	inputTerm, termOK := sourceDescriptorTerm(schema, input)
	outputBuilder, builderOK := schema.Builder(output)
	residual, residualOK := outputBuilder.Drop(inputTerm, bind.CellCount())
	if !inputOK || !outputOK || !sourceOK || !termOK || !builderOK || !residualOK {
		empty, emptyOK := outputBuilder.Closed()
		t.Fatalf("Bind law cold descriptors: input=%v output=%v source=%v term=%v builder=%v residual=%v inputTerm=%v empty=%v/%v", inputOK, outputOK, sourceOK, termOK, builderOK, residualOK, inputTerm.Kind(), empty.Kind(), emptyOK)
	}

	composition := engine.NewComposition()
	owner, ownerOK := packowner.Declare(composition, transformKey(20), schema)
	seed, seedOK := packsource.Declare(composition, transformKey(21), transformKey(22), transformKey(23), owner)
	operand, operandOK := NewBind(schema, bind)
	bindRule, bindRuleOK := DeclareBind(composition, transformKey(24), transformKey(25), transformKey(26), owner)
	var observed engine.QueryRead[engine.OrderedCells[packdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transformKey(27),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, observed)
				value, present, valueOK := cells.At(0)
				if !cellsOK || cells.Count() != 1 || !valueOK || !present || value.IsBottom() {
					return false
				}
				actual, actualOK := schema.Term(output, value)
				if !actualOK || !actual.Equal(residual) || (actual.Kind() == packdomain.TermOpen) != wantOpen {
					return false
				}
				for index := 0; index < bind.CellCount(); index++ {
					cell, cellOK := bind.CellAt(index)
					table, tableOK := schema.TableIndex(int64(index))
					want, wantOK := outputBuilder.ScalarAt(inputTerm, table)
					got, gotOK := schema.Scalar(output, value, cell)
					if !cellOK || !tableOK || !wantOK || !gotOK || got.Kind() != want.Kind() {
						return false
					}
				}
				return true
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: transformKey(28), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		observed, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !ownerOK || !seedOK || !operandOK || !bindRuleOK || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("Bind law composition")
	}
	seedInstance, seedInstanceOK := seed.Instance(inputSource)
	bindInstance, bindInstanceOK := bindRule.Instance(operand)
	if !seedInstanceOK || !bindInstanceOK {
		t.Fatal("Bind law instances")
	}
	result := testlaw.RunLinear(context.Background(), testlaw.LinearFixture[packdomain.Value, packdomain.Source, packdomain.Value, Bind, bool]{
		Composition: composition, Source: seedInstance, Steps: []*engine.RuleInstance[packdomain.Value, Bind]{bindInstance}, Query: query,
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			ref, refOK := owner.Locate(output)
			return refOK && engine.InstanceQueryRead(binding, observed, ref)
		},
		SourceSite: transformKey(29), SourceOccurrence: transformKey(30), StepSites: []engine.SemanticKey{transformKey(31)}, StepOccurrences: []engine.SemanticKey{transformKey(32)}, BoundarySemantics: []engine.SemanticKey{transformKey(33)},
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("Bind law solve: status=%v available=%v value=%v", result.Status, result.ValueAvailable, result.Value)
	}
}

func TestBindNilFillAndOpenTailRunThroughOneSourceAssembly(t *testing.T) {
	schema := transformBindSchema(t)
	closed, closedOK := bindDescriptorWithShape(schema.Link(), schema, 2, false)
	open, openOK := bindDescriptorWithShape(schema.Link(), schema, 2, true)
	if !closedOK || !openOK {
		t.Fatal("fixture did not expose closed and open width-two Binds")
	}
	runBindLaw(t, schema, closed, false)
	runBindLaw(t, schema, open, true)
}

func packSourceRoots(schema *packdomain.Schema) []packdomain.Root {
	if schema == nil {
		return nil
	}
	roots := make([]packdomain.Root, 0)
	for index := 0; index < schema.RootCount(); index++ {
		root, rootOK := schema.RootAt(index)
		if rootOK && schema.PackOnly(root) {
			if _, sourceOK := schema.Source(root); sourceOK {
				roots = append(roots, root)
			}
		}
	}
	return roots
}

func runSpliceLaw(t *testing.T, schema *packdomain.Schema, final bool) {
	t.Helper()
	roots := packSourceRoots(schema)
	if len(roots) < 3 {
		t.Fatal("splice fixture did not expose three Pack sources")
	}
	inputs := []packdomain.Root{roots[0], roots[1]}
	output := roots[2]
	terms := make([]packdomain.Term, len(inputs))
	for index, root := range inputs {
		term, termOK := sourceDescriptorTerm(schema, root)
		if !termOK {
			t.Fatal("splice source term")
		}
		terms[index] = term
	}
	outputBuilder, builderOK := schema.Builder(output)
	expected, expectedOK := outputBuilder.Splice(terms, final)
	if !builderOK || !expectedOK {
		t.Fatal("splice expected term")
	}

	composition := engine.NewComposition()
	owner, ownerOK := packowner.Declare(composition, transformKey(40), schema)
	seed0, seed0OK := packsource.Declare(composition, transformKey(41), transformKey(42), transformKey(43), owner)
	seed1, seed1OK := packsource.Declare(composition, transformKey(44), transformKey(45), transformKey(46), owner)
	operand, operandOK := NewSplice(schema, inputs, output, final)
	spliceRule, spliceRuleOK := DeclareSplice(composition, transformKey(47), transformKey(48), transformKey(49), owner, len(inputs))
	var observed engine.QueryRead[engine.OrderedCells[packdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transformKey(50),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, observed)
				value, present, valueOK := cells.At(0)
				term, termOK := schema.Term(output, value)
				return cellsOK && cells.Count() == 1 && valueOK && present && !value.IsBottom() && termOK && term.Equal(expected)
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: transformKey(51), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		observed, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !ownerOK || !seed0OK || !seed1OK || !operandOK || !spliceRuleOK || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("splice composition")
	}
	source0, source0OK := schema.Source(inputs[0])
	source1, source1OK := schema.Source(inputs[1])
	seed0Instance, seed0InstanceOK := seed0.Instance(source0)
	seed1Instance, seed1InstanceOK := seed1.Instance(source1)
	spliceInstance, spliceInstanceOK := spliceRule.Instance(operand)
	if !source0OK || !source1OK || !seed0InstanceOK || !seed1InstanceOK || !spliceInstanceOK {
		t.Fatal("splice instances")
	}

	source := engine.NewSourceAssembly(composition)
	if source == nil {
		t.Fatal("splice SourceAssembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falsityOK := source.FalseExpr()
	sourceSite0, sourceSite0OK := source.Site(transformKey(52), scope, truth, true)
	sourceSite1, sourceSite1OK := source.Site(transformKey(53), scope, truth, true)
	targetSite, targetSiteOK := source.Site(transformKey(54), scope, falsity, false)
	occurrence0, occurrence0OK := source.Relation(sourceSite0, transformKey(55))
	occurrence1, occurrence1OK := source.Relation(sourceSite1, transformKey(56))
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, transformKey(57))
	prepared0, prepared0OK := source.PrepareInstance(occurrence0, seed0Instance)
	prepared1, prepared1OK := source.PrepareInstance(occurrence1, seed1Instance)
	preparedTarget, preparedTargetOK := source.PrepareInstance(targetOccurrence, spliceInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary0, boundary0OK := source.Boundary(sourceSite0, targetSite, transformKey(58), truth, reindex, truth)
	boundary1, boundary1OK := source.Boundary(sourceSite1, targetSite, transformKey(59), truth, reindex, truth)
	if !scopeOK || !truthOK || !falsityOK || !sourceSite0OK || !sourceSite1OK || !targetSiteOK || !occurrence0OK || !occurrence1OK || !targetOccurrenceOK || !prepared0OK || !prepared1OK || !preparedTargetOK || !reindexOK || !boundary0OK || !boundary1OK || !source.Seal() {
		t.Fatal("splice source topology")
	}

	var queryInstance *engine.QueryInstance[bool]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint0, sourcePoint0OK := assembly.Point(sourceSite0)
		sourcePoint1, sourcePoint1OK := assembly.Point(sourceSite1)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		member0, member0OK := assembly.Member(sourcePoint0, prepared0)
		member1, member1OK := assembly.Member(sourcePoint1, prepared1)
		targetMember, targetMemberOK := assembly.Member(targetPoint, preparedTarget)
		sourceGroup0, sourceGroup0OK := assembly.Group(sourcePoint0, member0)
		sourceGroup1, sourceGroup1OK := assembly.Group(sourcePoint1, member1)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, targetMember)
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool {
			ref, refOK := owner.Locate(output)
			return refOK && engine.InstanceQueryRead(binding, observed, ref)
		})
		_, queryAttached := assembly.Query(targetPoint, queryInstance)
		boundary0Attached := assembly.Boundary(targetGroup, boundary0)
		boundary1Attached := assembly.Boundary(targetGroup, boundary1)
		if !sourcePoint0OK || !sourcePoint1OK || !targetPointOK || !member0OK || !member1OK || !targetMemberOK || !sourceGroup0OK || !sourceGroup1OK || !targetGroupOK || !sourceGroup0.Available() || !sourceGroup1.Available() || !queryOK || !queryAttached || !boundary0Attached || !boundary1Attached {
			t.Logf("splice assembly parts source0=%v source1=%v target=%v member0=%v member1=%v targetMember=%v group0=%v group1=%v targetGroup=%v sourceAvail0=%v sourceAvail1=%v query=%v attached=%v boundary0=%v boundary1=%v", sourcePoint0OK, sourcePoint1OK, targetPointOK, member0OK, member1OK, targetMemberOK, sourceGroup0OK, sourceGroup1OK, targetGroupOK, sourceGroup0.Available(), sourceGroup1.Available(), queryOK, queryAttached, boundary0Attached, boundary1Attached)
			return false
		}
		return true
	})
	if !assembled || solver == nil {
		t.Fatalf("splice source assembly: assembled=%v solver=%v queryInstance=%v", assembled, solver != nil, queryInstance != nil)
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	value, valueOK := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || !receiptOK || !valueOK || !value {
		t.Fatalf("splice solve: status=%v state=%v receipt=%v value=%v/%v", status, state != nil, receiptOK, value, valueOK)
	}
}

func TestOrderedSpliceFinalAndNonFinalRunThroughOneSourceAssembly(t *testing.T) {
	schema := transformSchema(t)
	runSpliceLaw(t, schema, false)
	runSpliceLaw(t, schema, true)
}

func TestOrderedMultiReturnSplicePreservesSingleOpenValuesRoot(t *testing.T) {
	schema := transformBodySchema(t)
	input, output, inputBuilder, outputBuilder := singleValuesRootBuilders(t, schema)
	base, baseOK := sourceDescriptorTerm(schema, input)
	if !baseOK || base.Kind() != packdomain.TermOpen || base.FixedCount() < 2 {
		t.Fatal("single Values root is not an ordered open list")
	}
	first, firstOK := base.FixedAt(0)
	second, secondOK := base.FixedAt(1)
	if !firstOK || !secondOK {
		t.Fatal("single Values root fixed positions")
	}
	rest, suffix, tailOK := base.Tail()
	if !tailOK {
		t.Fatal("single Values root open tail")
	}
	nonFinalAlternative := []packdomain.Term{}
	for _, scalar := range []packdomain.Scalar{first, second} {
		term, termOK := inputBuilder.Closed(scalar)
		if !termOK {
			t.Fatal("non-final position alternative")
		}
		nonFinalAlternative = append(nonFinalAlternative, term)
	}
	finalAlternative := []packdomain.Term{}
	for _, scalar := range []packdomain.Scalar{first, second} {
		term, termOK := inputBuilder.Open([]packdomain.Scalar{scalar}, rest, suffix)
		if !termOK {
			t.Fatal("final open-tail alternative")
		}
		finalAlternative = append(finalAlternative, term)
	}
	nonFinalFact := packAlternativeFact(t, schema, input, inputBuilder, nonFinalAlternative)
	finalFact := packAlternativeFact(t, schema, input, inputBuilder, finalAlternative)
	operand, operandOK := NewSplice(schema, []packdomain.Root{input, input}, output, true)
	if !operandOK {
		t.Fatal("single Values root Splice operand")
	}
	actual, actualOK := mapSplice(schema, operand, []packdomain.Value{nonFinalFact, finalFact})
	if !actualOK || actual.IsBottom() {
		t.Fatal("single Values root Splice result")
	}
	expected := schema.Bottom()
	for _, nonFinal := range nonFinalAlternative {
		for _, final := range finalAlternative {
			term, termOK := outputBuilder.Splice([]packdomain.Term{nonFinal, final}, true)
			value, valueOK := outputBuilder.PackTerm(term)
			if !termOK || !valueOK {
				t.Fatal("single Values root expected splice")
			}
			expected = schema.Lattice().Join(expected, value)
		}
	}
	if !schema.Lattice().Equal(actual, expected) {
		t.Fatal("single Values root lost authored position cross-product")
	}
	terms, termsOK := schema.Terms(output, actual)
	if !termsOK || len(terms) != 4 {
		t.Fatalf("single Values root output terms = %d, want 4", len(terms))
	}
	for _, term := range terms {
		if term.Kind() != packdomain.TermOpen || term.FixedCount() != 2 {
			t.Fatal("single Values root lost non-final scalar or final open tail")
		}
	}
}

func TestOrderedMultiReturnSpliceBranchAlternativesUnion(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "pack_call_input_law.lua", Text: []byte(`
local function branch(flag)
  if flag then
    return 1
  elseif not flag then
    return 2
  end
end
branch(true)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_call_input_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, typeOK := typeauthority.Seal(linked)
	if !typeOK {
		t.Fatal("seal type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	schema, schemaOK := packdomain.Seal(linked, statics)
	if !schemaOK || schema == nil {
		t.Fatal("Pack branch schema")
	}
	outcome, outcomeOK := branchReturnOutcome(schema)
	if !outcomeOK {
		t.Fatal("branch Body.Return outcome")
	}
	if outcome.ValuesCount() != 2 {
		t.Fatalf("branch Body.Return alternatives = %d, want 2", outcome.ValuesCount())
	}
	transfer, transferOK := NewOutcomeTransfer(schema, outcome)
	if !transferOK {
		t.Fatal("branch OutcomeTransfer operand")
	}
	facts := make([]packdomain.Value, outcome.ValuesCount())
	expected := schema.Bottom()
	outputRoot, outputRootOK := outcome.Root()
	outputBuilder, outputBuilderOK := schema.Builder(outputRoot)
	if !outputRootOK || !outputBuilderOK {
		t.Fatal("branch Outcome root builder")
	}
	for index := range facts {
		inputRoot, inputRootOK := outcome.ValuesRootAt(index)
		inputBuilder, inputBuilderOK := schema.Builder(inputRoot)
		term, termOK := sourceDescriptorTerm(schema, inputRoot)
		if !inputRootOK || !inputBuilderOK || !termOK {
			t.Fatal("branch Values alternative")
		}
		facts[index] = packAlternativeFact(t, schema, inputRoot, inputBuilder, []packdomain.Term{term})
		value, valueOK := outputBuilder.PackTerm(term)
		if !valueOK {
			t.Fatal("branch expected union term")
		}
		expected = schema.Lattice().Join(expected, value)
	}
	actual, actualOK := mapOutcomeTransfer(schema, transfer, facts)
	if !actualOK || actual.IsBottom() || !schema.Lattice().Equal(actual, expected) {
		t.Fatal("branch Values alternatives were not unioned")
	}
	terms, termsOK := schema.Terms(outputRoot, actual)
	if !termsOK || len(terms) != 2 {
		t.Fatalf("branch Outcome terms = %d, want 2 independent alternatives", len(terms))
	}
	for _, term := range terms {
		if term.FixedCount() != 1 || term.Kind() != packdomain.TermClosed {
			t.Fatal("branch alternatives were cross-spliced")
		}
	}
}

type stagedBottomMismatchDeclaration struct {
	rule    *engine.Rule[packdomain.Value, OutcomeTransfer]
	checker *OutcomeTransferRule
	summary engine.Read[engine.OrderedCells[packdomain.Value]]
	write   engine.Write[packdomain.Value]
	owner   *packowner.Owner
}

func (declaration *stagedBottomMismatchDeclaration) Instance(operand OutcomeTransfer) (*engine.RuleInstance[packdomain.Value, OutcomeTransfer], bool) {
	if declaration == nil || declaration.rule == nil || declaration.owner == nil {
		return nil, false
	}
	refs := outcomeSummaryRefs(declaration.owner, operand)
	outputRef, outputOK := declaration.owner.Locate(operand.OutcomeRoot())
	if refs == nil || !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, OutcomeTransfer]) bool {
		return packowner.InstanceSummaryRead(declaration.owner, binding, declaration.summary, refs) && engine.InstanceWrite(binding, declaration.write, outputRef)
	})
}

func declareStagedBottomMismatch(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner, operand OutcomeTransfer) (*stagedBottomMismatchDeclaration, bool) {
	checker := &OutcomeTransferRule{semantic: semantic, owner: owner}
	var summary engine.Read[engine.OrderedCells[packdomain.Value]]
	var write engine.Write[packdomain.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, OutcomeTransfer]{
		Semantic: semantic, OperandFamily: family, OperandContent: checker.operandContent,
		Output: owner.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation(evidence, checker.check),
		Transfer: func(access engine.Access[packdomain.Value, OutcomeTransfer]) bool {
			return engine.Product(access, func(row engine.Row) bool {
				cells, cellsOK := engine.ReadValue(access, row, summary)
				if !cellsOK || cells.Count() != operand.InputCount() {
					return false
				}
				return engine.StageValue(access, row, owner.Schema().Top())
			})
		},
	}, func(rule *engine.Rule[packdomain.Value, OutcomeTransfer]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		summary, readOK = engine.ReadFrom(rule, input, owner.SummaryRead())
		write, writeOK = engine.WriteTo(rule, owner.ExactWrite())
		checker.summary = summary
		return inputOK && readOK && writeOK
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &stagedBottomMismatchDeclaration{rule: declared, checker: checker, summary: summary, write: write, owner: owner}, true
}

func assembleOutcomeTransferBottom(t testing.TB, malformed bool) (*engine.Solver, bool) {
	t.Helper()
	schema := outcomeTransferLawSchema(t)
	outcome, outcomeOK := branchReturnOutcome(schema)
	if !outcomeOK {
		t.Fatal("bottom OutcomeReturn fixture")
	}
	operand, operandOK := NewOutcomeTransfer(schema, outcome)
	if !operandOK {
		t.Fatal("bottom OutcomeTransfer operand")
	}
	composition := engine.NewComposition()
	owner, ownerOK := packowner.DeclareWithSummary(composition, transformKey(200), transformKey(201), schema)
	if !ownerOK {
		t.Fatal("bottom Pack owner")
	}
	transferSemantic, transferFamily, transferEvidence := transformKey(202), transformKey(203), transformKey(204)
	var transferRule *OutcomeTransferRule
	var transferDeclaration *stagedBottomMismatchDeclaration
	var transferInstance *engine.RuleInstance[packdomain.Value, OutcomeTransfer]
	if malformed {
		transferDeclaration, operandOK = declareStagedBottomMismatch(composition, transferSemantic, transferFamily, transferEvidence, owner, operand)
	} else {
		transferRule, operandOK = DeclareOutcomeTransfer(composition, transferSemantic, transferFamily, transferEvidence, owner)
	}
	seedDeclarations := make([]*bottomSummarySeedDeclaration, operand.InputCount())
	for index := range seedDeclarations {
		seed, seedOK := declareBottomSummarySeed(composition, transformKey(byte(210+index*3)), transformKey(byte(211+index*3)), transformKey(byte(212+index*3)), owner)
		if !seedOK {
			t.Fatal("bottom SummaryRead seed")
		}
		seedDeclarations[index] = seed
	}
	var observed engine.QueryRead[engine.OrderedCells[packdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transformKey(253),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, observed)
				return cellsOK && cells.Count() == 1
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{Semantic: transformKey(254), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value }, Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		}},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		observed, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !queryOK || query == nil {
		t.Fatal("bottom OutcomeTransfer query")
	}
	if !composition.Seal() {
		t.Fatal("bottom OutcomeTransfer composition")
	}
	if malformed {
		transferInstance, operandOK = transferDeclaration.Instance(operand)
	} else {
		transferInstance, operandOK = transferRule.Instance(operand)
	}
	if !operandOK || transferInstance == nil {
		t.Fatal("bottom OutcomeTransfer instance")
	}
	seedInstances := make([]*engine.RuleInstance[packdomain.Value, bottomSummarySeed], len(seedDeclarations))
	for index, seed := range seedDeclarations {
		inputRoot, inputRootOK := operand.InputAt(index)
		var seedOK bool
		seedInstances[index], seedOK = seed.Instance(inputRoot, bottomSummarySeed{id: transformKey(byte(213 + index)).Digest()})
		if !inputRootOK || !seedOK {
			t.Fatal("bottom SummaryRead seed instance")
		}
	}
	source := engine.NewSourceAssembly(composition)
	if source == nil {
		t.Fatal("bottom OutcomeTransfer SourceAssembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falsityOK := source.FalseExpr()
	inputSites := make([]engine.SourceSite, len(seedInstances))
	inputSitesOK := true
	for index := range inputSites {
		inputSites[index], inputSitesOK = source.Site(transformKey(byte(220+index)), scope, truth, true)
		if !inputSitesOK {
			break
		}
	}
	targetSite, targetSiteOK := source.Site(transformKey(230), scope, falsity, false)
	preparedSeeds := make([]engine.SourceInstance, len(seedInstances))
	preparedSeedsOK := true
	for index, seedInstance := range seedInstances {
		occurrence, occurrenceOK := source.Relation(inputSites[index], transformKey(byte(240+index)))
		preparedSeeds[index], occurrenceOK = source.PrepareInstance(occurrence, seedInstance)
		preparedSeedsOK = preparedSeedsOK && occurrenceOK
	}
	transferOccurrence, transferOccurrenceOK := source.Relation(targetSite, transformKey(250))
	preparedTransfer, preparedTransferOK := source.PrepareInstance(transferOccurrence, transferInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundaries := make([]engine.SourceBoundary, 1)
	var boundariesOK bool
	boundaries[0], boundariesOK = source.Boundary(inputSites[0], targetSite, transformKey(1), truth, reindex, truth)
	if !scopeOK || !truthOK || !falsityOK || !inputSitesOK || !targetSiteOK || !preparedSeedsOK || !transferOccurrenceOK || !preparedTransferOK || !reindexOK || !boundariesOK || !source.Seal() {
		t.Fatal("bottom OutcomeTransfer source topology")
	}
	var queryInstance *engine.QueryInstance[bool]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		inputPoints := make([]engine.AssemblyPoint, len(inputSites))
		inputPointsOK := true
		for index, site := range inputSites {
			inputPoints[index], inputPointsOK = assembly.Point(site)
			if !inputPointsOK {
				break
			}
		}
		targetPoint, targetPointOK := assembly.Point(targetSite)
		seedMembers := make([]engine.AssemblyMember, len(preparedSeeds))
		seedMembersOK := true
		for index, prepared := range preparedSeeds {
			seedMembers[index], seedMembersOK = assembly.Member(inputPoints[index], prepared)
			seedMembersOK = seedMembersOK && seedMembers[index].Available()
		}
		transferMember, transferMemberOK := assembly.Member(targetPoint, preparedTransfer)
		inputGroupsOK := true
		for index, member := range seedMembers {
			_, groupOK := assembly.Group(inputPoints[index], member)
			inputGroupsOK = inputGroupsOK && groupOK
		}
		targetGroup, targetGroupOK := assembly.Group(targetPoint, transferMember)
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool {
			ref, refOK := owner.Locate(operand.OutcomeRoot())
			return refOK && engine.InstanceQueryRead(binding, observed, ref)
		})
		_, queryAttached := assembly.Query(targetPoint, queryInstance)
		boundariesAttached := true
		boundariesAttached = assembly.Boundary(targetGroup, boundaries[0])
		return inputPointsOK && targetPointOK && seedMembersOK && transferMemberOK && inputGroupsOK && targetGroupOK && queryOK && queryAttached && boundariesAttached
	})
	if !assembled || solver == nil || queryInstance == nil {
		t.Fatal("bottom OutcomeTransfer source assembly")
	}
	return solver, true
}

func TestOutcomeTransferAllBottomSummaryReadUsesNoCandidate(t *testing.T) {
	solver, ok := assembleOutcomeTransferBottom(t, false)
	if !ok {
		t.Fatal("bottom OutcomeTransfer assembly")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("bottom OutcomeTransfer solve: status=%v state=%v", status, state != nil)
	}
}

func TestOutcomeTransferRejectsStagedBottomMismatch(t *testing.T) {
	solver, ok := assembleOutcomeTransferBottom(t, true)
	if !ok {
		t.Fatal("mismatch OutcomeTransfer assembly")
	}
	state, status := solver.Solve(context.Background())
	if state != nil || status != engine.SolveIncomplete {
		t.Fatalf("staged bottom mismatch was admitted: status=%v state=%v", status, state != nil)
	}
}

func singleValuesRootBuilders(t testing.TB, schema *packdomain.Schema) (packdomain.Root, packdomain.Root, packdomain.Builder, packdomain.Builder) {
	t.Helper()
	if schema == nil || schema.Link() == nil || schema.Link().Project() == nil {
		t.Fatal("single Values schema")
	}
	mounts := schema.Link().Project().Mounts()
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		program, programOK := mounts.Program(shard)
		if !shardOK || !programOK || program == nil {
			continue
		}
		identity := program.Source().Identity()
		for ordinal := 1; ordinal <= identity.FamilyCount(keyspace.FamilyBody); ordinal++ {
			bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
			if !program.Flow().Executable().Contains(bodyTerm) {
				continue
			}
			body, bodyOK := schema.Body(shard, bodyTerm)
			outcome, outcomeOK := body.Return()
			if !bodyOK || !outcomeOK || outcome.ValuesCount() != 1 {
				continue
			}
			input, inputOK := outcome.ValuesRootAt(0)
			output, outputOK := outcome.Root()
			inputBuilder, inputBuilderOK := schema.Builder(input)
			outputBuilder, outputBuilderOK := schema.Builder(output)
			if inputOK && outputOK && inputBuilderOK && outputBuilderOK {
				term, termOK := sourceDescriptorTerm(schema, input)
				if termOK && term.Kind() == packdomain.TermOpen && term.FixedCount() >= 2 {
					return input, output, inputBuilder, outputBuilder
				}
			}
		}
	}
	t.Fatal("single ordered Values root")
	return packdomain.Root{}, packdomain.Root{}, packdomain.Builder{}, packdomain.Builder{}
}

func packAlternativeFact(t testing.TB, schema *packdomain.Schema, root packdomain.Root, builder packdomain.Builder, terms []packdomain.Term) packdomain.Value {
	t.Helper()
	port, portOK := schema.Source(root)
	item, itemOK := port.At(0)
	whole, wholeOK := item.Port()
	if !portOK || !itemOK || !wholeOK {
		t.Fatal("Pack alternative source port")
	}
	cases := make([]packdomain.Case, 0, len(terms))
	for _, term := range terms {
		equation, equationOK := builder.Pack(whole, term)
		caseValue, caseOK := builder.Case(equation)
		if !equationOK || !caseOK {
			t.Fatal("Pack alternative case")
		}
		cases = append(cases, caseValue)
	}
	fact, factOK := builder.Value(cases...)
	if !factOK || fact.IsBottom() {
		t.Fatal("Pack alternative fact")
	}
	return fact
}

func branchReturnOutcome(schema *packdomain.Schema) (packdomain.Outcome, bool) {
	if schema == nil || schema.Link() == nil || schema.Link().Project() == nil {
		return packdomain.Outcome{}, false
	}
	mounts := schema.Link().Project().Mounts()
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		program, programOK := mounts.Program(shard)
		if !shardOK || !programOK || program == nil {
			continue
		}
		identity := program.Source().Identity()
		for ordinal := 1; ordinal <= identity.FamilyCount(keyspace.FamilyBody); ordinal++ {
			bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
			if !program.Flow().Executable().Contains(bodyTerm) {
				continue
			}
			body, bodyOK := schema.Body(shard, bodyTerm)
			outcome, outcomeOK := body.Return()
			if bodyOK && outcomeOK && outcome.ValuesCount() >= 2 {
				return outcome, true
			}
		}
	}
	return packdomain.Outcome{}, false
}

func bodyCallValue(schema *packdomain.Schema, calls *calldomain.Algebra, application linkproject.Application, body calldomain.Body) (calldomain.Value, bool) {
	if schema == nil || calls == nil || schema.Link() != calls.Link() || !body.Valid() {
		return calldomain.Value{}, false
	}
	key, keyOK := calls.KeyForApplication(application)
	shard, bodyTerm, bodyOK := calls.ResolveBody(body)
	bodyDesc, bodyDescOK := schema.Body(shard, bodyTerm)
	function, functionOK := bodyDesc.Function()
	target, targetOK := calls.TargetForFunction(shard, function)
	if !keyOK || !bodyOK || !bodyDescOK || !functionOK || !targetOK {
		return calldomain.Value{}, false
	}
	value, valueOK := calls.DispatchValue(key, []calldomain.Target{target}, false)
	return value, valueOK && callValueHasBody(value, body)
}

func bodyReturnTerm(schema *packdomain.Schema, calls *calldomain.Algebra, body calldomain.Body) (packdomain.Root, packdomain.Term, bool) {
	if schema == nil || calls == nil || schema.Link() != calls.Link() || !body.Valid() {
		return packdomain.Root{}, packdomain.Term{}, false
	}
	shard, bodyTerm, bodyOK := calls.ResolveBody(body)
	bodyDesc, bodyDescOK := schema.Body(shard, bodyTerm)
	returned, returnOK := bodyDesc.Return()
	outcomeRoot, outcomeRootOK := returned.Root()
	if !bodyOK || !bodyDescOK || !returnOK || !outcomeRootOK {
		return packdomain.Root{}, packdomain.Term{}, false
	}
	var valuesTerm keyspace.Term
	if returned.ValuesCount() > 0 {
		valuesTerm, _ = returned.ValuesTermAt(0)
	}
	if valuesTerm == 0 {
		builder, builderOK := schema.Builder(outcomeRoot)
		term, termOK := builder.Closed()
		return outcomeRoot, term, builderOK && termOK
	}
	_, valuesRoot, valuesOK := schema.Values(shard, valuesTerm)
	term, termOK := sourceDescriptorTerm(schema, valuesRoot)
	return outcomeRoot, term, valuesOK && termOK
}

func TestBodyEntryRunsThroughOneSourceAssembly(t *testing.T) {
	schema := transformBodySchema(t)
	callsAlg, callsOK := calldomain.New(schema.Link())
	if !callsOK {
		t.Fatal("Call algebra")
	}
	applications := schema.Link().Project().Applications().Calls()
	var application linkproject.Application
	var body calldomain.Body
	var operand BodyEntry
	var operandOK bool
	for applicationIndex := 0; applicationIndex < applications.Count() && !operandOK; applicationIndex++ {
		candidateApplication, candidateApplicationOK := applications.At(applicationIndex)
		if !candidateApplicationOK {
			continue
		}
		for bodyIndex := 0; bodyIndex < callsAlg.Bodies().Count(); bodyIndex++ {
			candidateBody, candidateBodyOK := callsAlg.Bodies().At(bodyIndex)
			candidateOperand, candidateOperandOK := NewBodyEntry(schema, callsAlg, candidateApplication, candidateBody)
			if candidateBodyOK && candidateOperandOK {
				if _, callValueOK := bodyCallValue(schema, callsAlg, candidateApplication, candidateBody); callValueOK {
					application, body, operand, operandOK = candidateApplication, candidateBody, candidateOperand, true
					break
				}
			}
		}
	}
	if !operandOK {
		t.Fatal("direct Body entry operand")
	}
	callRoot := operand.CallRoot()
	bodyRoot, bodyRootOK := operand.packBody.Root()
	if !bodyRootOK {
		t.Fatal("body root projection")
	}
	_, callRootOK := schema.RootID(callRoot)
	_, bodyRootIDOK := schema.RootID(bodyRoot)
	callTerm, callTermOK := sourceDescriptorTerm(schema, callRoot)
	bodyShard, bodyTerm, bodyResolved := callsAlg.ResolveBody(body)
	bodyDesc, bodyDescOK := schema.Body(bodyShard, bodyTerm)
	bodyBuilder, bodyBuilderOK := schema.Builder(bodyRoot)
	residual, residualOK := bodyBuilder.Drop(callTerm, bodyDesc.FormalCount())
	_, residualTableOK := schema.TableIndex(int64(bodyDesc.FormalCount()))
	wantScalars := make([]packdomain.Scalar, bodyDesc.FormalCount())
	for index := range wantScalars {
		table, tableOK := schema.TableIndex(int64(index))
		var scalarOK bool
		wantScalars[index], scalarOK = bodyBuilder.ScalarAt(callTerm, table)
		if !tableOK || !scalarOK {
			t.Fatal("body-entry formal selector")
		}
	}
	if len(wantScalars) == 0 {
		t.Fatal("body-entry has no formal descriptor")
	}
	firstSourceScalar, firstSourceOK := callTerm.FixedAt(0)
	firstSourceEndpoint, firstSourceEndpointOK := firstSourceScalar.Endpoint()
	firstFormal, firstFormalOK := bodyDesc.FormalAt(0)
	firstFormalWant, firstFormalWantOK := wantScalars[0].Endpoint()
	if !operandOK || !callRootOK || !bodyRootOK || !bodyRootIDOK || !bodyResolved || !callTermOK || !bodyDescOK || !bodyBuilderOK || !residualOK || !residualTableOK || bodyDesc.FormalCount() < 3 || callTerm.FixedCount() != 1 || !firstSourceOK || !firstSourceEndpointOK || !firstFormalOK || !firstFormalWantOK || firstSourceEndpoint == firstFormal || firstSourceEndpoint != firstFormalWant || residual.Kind() != packdomain.TermOpen {
		t.Fatal("body-entry exact formal/vararg shape")
	}
	callValue, callValueOK := bodyCallValue(schema, callsAlg, application, body)
	if !callValueOK {
		t.Fatal("body-entry Call Body value")
	}

	composition := engine.NewComposition()
	packs, packsOK := packowner.Declare(composition, transformKey(60), schema)
	calls, callsOwnerOK := callowner.Declare(composition, transformKey(61), callsAlg)
	seed, seedOK := packsource.Declare(composition, transformKey(62), transformKey(63), transformKey(64), packs)
	bodyRule, bodyRuleOK := DeclareBodyEntry(composition, transformKey(65), transformKey(66), transformKey(67), packs, calls)
	var callWrite engine.Write[calldomain.Value]
	callSeed, callSeedOK := engine.DeclareRule(composition, engine.RuleSpec[calldomain.Value, bodyLawSeed]{
		Semantic: transformKey(68), OperandFamily: transformKey(69), OperandContent: bodyLawSeedContent,
		Output: calls.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[calldomain.Value, bodyLawSeed](transformKey(70)),
		Transfer: func(access engine.Access[calldomain.Value, bodyLawSeed]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, callValue) })
		},
	}, func(rule *engine.Rule[calldomain.Value, bodyLawSeed]) bool {
		var declared bool
		callWrite, declared = engine.WriteTo(rule, calls.ExactWrite())
		return declared
	})
	var observed engine.QueryRead[engine.OrderedCells[packdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transformKey(71),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, observed)
				value, present, valueOK := cells.At(0)
				if !cellsOK || cells.Count() != 1 || !present || !valueOK || value.IsBottom() {
					return false
				}
				term, termOK := schema.Term(bodyRoot, value)
				if !termOK || !term.Equal(residual) {
					return false
				}
				for index, want := range wantScalars {
					endpoint, endpointOK := bodyDesc.FormalAt(index)
					got, gotOK := schema.Scalar(bodyRoot, value, endpoint)
					if !endpointOK || !gotOK || got.Kind() != want.Kind() {
						return false
					}
					wantEndpoint, wantEndpointOK := want.Endpoint()
					gotEndpoint, gotEndpointOK := got.Endpoint()
					if wantEndpointOK != gotEndpointOK || (wantEndpointOK && wantEndpoint != gotEndpoint) {
						return false
					}
				}
				return true
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: transformKey(72), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		observed, declared = engine.QueryReadFrom(query, packs.ExactRead())
		return declared
	})
	if !packsOK || !callsOwnerOK || !seedOK || !bodyRuleOK || !callSeedOK || callSeed == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("body-entry composition")
	}
	inputSource, inputSourceOK := schema.Source(callRoot)
	callKey, callKeyOK := callsAlg.KeyForApplication(application)
	callRef, callRefOK := calls.Locate(callKey)
	seedInstance, seedInstanceOK := seed.Instance(inputSource)
	callInstance, callInstanceOK := engine.NewRuleInstance(callSeed, bodyLawSeed{id: transformKey(73).Digest()}, func(binding *engine.RuleBinding[calldomain.Value, bodyLawSeed]) bool {
		return engine.InstanceWrite(binding, callWrite, callRef)
	})
	bodyInstance, bodyInstanceOK := bodyRule.Instance(operand)
	if !inputSourceOK || !callKeyOK || !callRefOK || !seedInstanceOK || !callInstanceOK || !bodyInstanceOK {
		t.Fatal("body-entry instances")
	}

	source := engine.NewSourceAssembly(composition)
	if source == nil {
		t.Fatal("body-entry SourceAssembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falsityOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(transformKey(74), scope, truth, true)
	targetSite, targetSiteOK := source.Site(transformKey(75), scope, falsity, false)
	callOccurrence, callOccurrenceOK := source.At(sourceSite)
	packOccurrence, packOccurrenceOK := source.Relation(sourceSite, transformKey(76))
	bodyOccurrence, bodyOccurrenceOK := source.Relation(targetSite, transformKey(77))
	preparedCall, preparedCallOK := source.PrepareInstance(callOccurrence, callInstance)
	preparedPack, preparedPackOK := source.PrepareInstance(packOccurrence, seedInstance)
	preparedBody, preparedBodyOK := source.PrepareInstance(bodyOccurrence, bodyInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(sourceSite, targetSite, transformKey(78), truth, reindex, truth)
	if !scopeOK || !truthOK || !falsityOK || !sourceSiteOK || !targetSiteOK || !callOccurrenceOK || !packOccurrenceOK || !bodyOccurrenceOK || !preparedCallOK || !preparedPackOK || !preparedBodyOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("body-entry source topology")
	}
	var queryInstance *engine.QueryInstance[bool]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		callMember, callMemberOK := assembly.Member(sourcePoint, preparedCall)
		packMember, packMemberOK := assembly.Member(sourcePoint, preparedPack)
		bodyMember, bodyMemberOK := assembly.Member(targetPoint, preparedBody)
		sourceGroup, sourceGroupOK := assembly.Group(sourcePoint, callMember, packMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, bodyMember)
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool {
			ref, refOK := packs.Locate(bodyRoot)
			return refOK && engine.InstanceQueryRead(binding, observed, ref)
		})
		_, queryAttached := assembly.Query(targetPoint, queryInstance)
		boundaryAttached := assembly.Boundary(targetGroup, boundary)
		return sourcePointOK && targetPointOK && callMemberOK && packMemberOK && bodyMemberOK && sourceGroupOK && targetGroupOK && sourceGroup.Available() && queryOK && queryAttached && boundaryAttached
	})
	if !assembled || solver == nil {
		t.Fatal("body-entry source assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	value, valueOK := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || !receiptOK || !valueOK || !value {
		t.Fatalf("body-entry solve: status=%v state=%v receipt=%v value=%v/%v", status, state != nil, receiptOK, value, valueOK)
	}
}

func TestBodyNormalReturnRunsThroughOneSourceAssembly(t *testing.T) {
	schema := transformBodySchema(t)
	callsAlg, callsOK := calldomain.New(schema.Link())
	if !callsOK {
		t.Fatal("Call algebra")
	}
	applications := schema.Link().Project().Applications().Calls()
	var application linkproject.Application
	var body calldomain.Body
	var operand BodyReturn
	var operandOK bool
	for applicationIndex := 0; applicationIndex < applications.Count() && !operandOK; applicationIndex++ {
		candidateApplication, candidateApplicationOK := applications.At(applicationIndex)
		if !candidateApplicationOK {
			continue
		}
		for bodyIndex := 0; bodyIndex < callsAlg.Bodies().Count(); bodyIndex++ {
			candidate, candidateOK := callsAlg.Bodies().At(bodyIndex)
			candidateOperand, candidateOperandOK := NewBodyReturn(schema, callsAlg, candidateApplication, candidate)
			if candidateOK && candidateOperandOK {
				if _, callValueOK := bodyCallValue(schema, callsAlg, candidateApplication, candidate); callValueOK {
					application, body, operand, operandOK = candidateApplication, candidate, candidateOperand, true
					break
				}
			}
		}
	}
	if !operandOK {
		t.Fatal("direct Body return operand")
	}
	callValue, callValueOK := bodyCallValue(schema, callsAlg, application, body)
	outcomeRoot, returnTerm, returnOK := bodyReturnTerm(schema, callsAlg, body)
	if !callValueOK || !returnOK || outcomeRoot != operand.OutcomeRoot() || returnTerm.Kind() != packdomain.TermOpen {
		t.Fatal("body-return exact normal term")
	}
	composition := engine.NewComposition()
	packs, packsOK := packowner.DeclareWithSummary(composition, transformKey(80), transformKey(108), schema)
	calls, callsOwnerOK := callowner.Declare(composition, transformKey(81), callsAlg)
	bodyRule, bodyRuleOK := DeclareBodyReturn(composition, transformKey(82), transformKey(83), transformKey(84), packs, calls)
	var callWrite engine.Write[calldomain.Value]
	callSeed, callSeedOK := engine.DeclareRule(composition, engine.RuleSpec[calldomain.Value, bodyLawSeed]{
		Semantic: transformKey(85), OperandFamily: transformKey(86), OperandContent: bodyLawSeedContent,
		Output: calls.Output(), Inputs: 0, Admission: engine.AdmitRuleByTrustedTheorem[calldomain.Value, bodyLawSeed](transformKey(87)),
		Transfer: func(access engine.Access[calldomain.Value, bodyLawSeed]) bool {
			return engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, callValue) })
		},
	}, func(rule *engine.Rule[calldomain.Value, bodyLawSeed]) bool {
		var declared bool
		callWrite, declared = engine.WriteTo(rule, calls.ExactWrite())
		return declared
	})
	transferOperand, transferOperandOK := NewOutcomeTransfer(schema, operand.outcome)
	outcomeTransfer, outcomeTransferOK := DeclareOutcomeTransfer(composition, transformKey(88), transformKey(89), transformKey(90), packs)
	inputSeeds := make([]*packsource.Rule, transferOperand.InputCount())
	inputSources := make([]packdomain.Source, transferOperand.InputCount())
	for index := range inputSeeds {
		inputRoot, inputRootOK := transferOperand.InputAt(index)
		inputSources[index], inputRootOK = schema.Source(inputRoot)
		seed, seedOK := packsource.Declare(composition, transformKey(110+byte(index*3)), transformKey(111+byte(index*3)), transformKey(112+byte(index*3)), packs)
		if !inputRootOK || !seedOK {
			transferOperandOK = false
			continue
		}
		inputSeeds[index] = seed
	}
	var observed engine.QueryRead[engine.OrderedCells[packdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: transformKey(91),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, observed)
				value, present, valueOK := cells.At(0)
				if !cellsOK || cells.Count() != 1 || !present || !valueOK || value.IsBottom() {
					return false
				}
				term, termOK := schema.Term(operand.CallRoot(), value)
				return termOK && term.Equal(returnTerm)
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: transformKey(92), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		observed, declared = engine.QueryReadFrom(query, packs.ExactRead())
		return declared
	})
	if !packsOK || !callsOwnerOK || !bodyRuleOK || !callSeedOK || callSeed == nil || !transferOperandOK || !outcomeTransferOK || outcomeTransfer == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("body-return composition")
	}
	callKey, callKeyOK := callsAlg.KeyForApplication(application)
	callRef, callRefOK := calls.Locate(callKey)
	callInstance, callInstanceOK := engine.NewRuleInstance(callSeed, bodyLawSeed{id: transformKey(93).Digest()}, func(binding *engine.RuleBinding[calldomain.Value, bodyLawSeed]) bool {
		return engine.InstanceWrite(binding, callWrite, callRef)
	})
	inputInstances := make([]*engine.RuleInstance[packdomain.Value, packdomain.Source], len(inputSeeds))
	inputInstancesOK := true
	for index, seed := range inputSeeds {
		instance, instanceOK := seed.Instance(inputSources[index])
		inputInstances[index] = instance
		inputInstancesOK = inputInstancesOK && instanceOK
	}
	outcomeInstance, outcomeInstanceOK := outcomeTransfer.Instance(transferOperand)
	bodyInstance, bodyInstanceOK := bodyRule.Instance(operand)
	if !callKeyOK || !callRefOK || !callInstanceOK || !inputInstancesOK || !outcomeInstanceOK || !bodyInstanceOK {
		t.Fatal("body-return instances")
	}

	source := engine.NewSourceAssembly(composition)
	if source == nil {
		t.Fatal("body-return SourceAssembly")
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falsityOK := source.FalseExpr()
	sourceSites := make([]engine.SourceSite, len(inputInstances))
	sourceSitesOK := true
	for index := range sourceSites {
		sourceSites[index], sourceSitesOK = source.Site(transformKey(100+byte(index)), scope, truth, true)
		if !sourceSitesOK {
			break
		}
	}
	outcomeSite, outcomeSiteOK := source.Site(transformKey(140), scope, truth, true)
	targetSite, targetSiteOK := source.Site(transformKey(96), scope, falsity, false)
	var callOccurrence engine.SourceOccurrence
	callOccurrenceOK := false
	if len(sourceSites) > 0 {
		callOccurrence, callOccurrenceOK = source.At(sourceSites[0])
	}
	outcomeOccurrence, outcomeOccurrenceOK := source.Relation(outcomeSite, transformKey(141))
	bodyOccurrence, bodyOccurrenceOK := source.Relation(targetSite, transformKey(98))
	preparedCall, preparedCallOK := source.PrepareInstance(callOccurrence, callInstance)
	preparedOutcome, preparedOutcomeOK := source.PrepareInstance(outcomeOccurrence, outcomeInstance)
	preparedInputs := make([]engine.SourceInstance, len(inputInstances))
	preparedInputsOK := true
	for index, instance := range inputInstances {
		occurrence, occurrenceOK := source.Relation(sourceSites[index], transformKey(120+byte(index)))
		prepared, preparedOK := source.PrepareInstance(occurrence, instance)
		preparedInputs[index] = prepared
		preparedInputsOK = preparedInputsOK && occurrenceOK && preparedOK
	}
	preparedBody, preparedBodyOK := source.PrepareInstance(bodyOccurrence, bodyInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	sourceToOutcome := make([]engine.SourceBoundary, len(sourceSites))
	sourceToOutcomeOK := true
	for index, site := range sourceSites {
		sourceToOutcome[index], sourceToOutcomeOK = source.Boundary(site, outcomeSite, transformKey(150+byte(index)), truth, reindex, truth)
		if !sourceToOutcomeOK {
			break
		}
	}
	callToTarget, callToTargetOK := source.Boundary(sourceSites[0], targetSite, transformKey(179), truth, reindex, truth)
	outcomeToTarget, outcomeToTargetOK := source.Boundary(outcomeSite, targetSite, transformKey(180), truth, reindex, truth)
	if !scopeOK || !truthOK || !falsityOK || !sourceSitesOK || !outcomeSiteOK || !targetSiteOK || !callOccurrenceOK || !outcomeOccurrenceOK || !bodyOccurrenceOK || !preparedCallOK || !preparedOutcomeOK || !preparedInputsOK || !preparedBodyOK || !reindexOK || !sourceToOutcomeOK || !callToTargetOK || !outcomeToTargetOK || !source.Seal() {
		t.Fatal("body-return source topology")
	}
	var queryInstance *engine.QueryInstance[bool]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoints := make([]engine.AssemblyPoint, len(sourceSites))
		sourcePointsOK := true
		for index, site := range sourceSites {
			sourcePoints[index], sourcePointsOK = assembly.Point(site)
			if !sourcePointsOK {
				break
			}
		}
		outcomePoint, outcomePointOK := assembly.Point(outcomeSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		callMember, callMemberOK := assembly.Member(sourcePoints[0], preparedCall)
		inputMembers := make([]engine.AssemblyMember, len(preparedInputs))
		inputMembersOK := true
		for index, prepared := range preparedInputs {
			member, memberOK := assembly.Member(sourcePoints[index], prepared)
			inputMembers[index] = member
			inputMembersOK = inputMembersOK && memberOK
		}
		outcomeMember, outcomeMemberOK := assembly.Member(outcomePoint, preparedOutcome)
		bodyMember, bodyMemberOK := assembly.Member(targetPoint, preparedBody)
		sourceGroups := make([]engine.AssemblyGroup, len(inputMembers))
		sourceGroupsOK := true
		for index, inputMember := range inputMembers {
			members := []engine.AssemblyMember{inputMember}
			if index == 0 {
				members = append(members, callMember)
			}
			sourceGroups[index], sourceGroupsOK = assembly.Group(sourcePoints[index], members...)
			if !sourceGroupsOK {
				break
			}
		}
		outcomeGroup, outcomeGroupOK := assembly.Group(outcomePoint, outcomeMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, bodyMember)
		queryInstance, queryOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool {
			ref, refOK := packs.Locate(operand.CallRoot())
			return refOK && engine.InstanceQueryRead(binding, observed, ref)
		})
		_, queryAttached := assembly.Query(targetPoint, queryInstance)
		sourceBoundariesAttached := true
		for index := range sourceGroups {
			sourceBoundariesAttached = sourceBoundariesAttached && assembly.Boundary(outcomeGroup, sourceToOutcome[index])
		}
		callBoundaryAttached := assembly.Boundary(targetGroup, callToTarget)
		outcomeBoundaryAttached := assembly.Boundary(targetGroup, outcomeToTarget)
		return sourcePointsOK && outcomePointOK && targetPointOK && callMemberOK && outcomeMemberOK && inputMembersOK && bodyMemberOK && sourceGroupsOK && outcomeGroupOK && targetGroupOK && queryOK && queryAttached && sourceBoundariesAttached && callBoundaryAttached && outcomeBoundaryAttached
	})
	if !assembled || solver == nil {
		t.Fatal("body-return source assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	value, valueOK := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || state == nil || !receiptOK || !valueOK || !value {
		t.Fatalf("body-return solve: status=%v state=%v receipt=%v value=%v/%v", status, state != nil, receiptOK, value, valueOK)
	}
}

func TestBodyReturnRejectsOrdinaryNonTailCallResult(t *testing.T) {
	schema := transformBodySchemaMode(t, false)
	callsAlg, callsOK := calldomain.New(schema.Link())
	if !callsOK {
		t.Fatal("Call algebra")
	}
	applications := schema.Link().Project().Applications().Calls()
	for applicationIndex := 0; applicationIndex < applications.Count(); applicationIndex++ {
		application, applicationOK := applications.At(applicationIndex)
		if !applicationOK {
			continue
		}
		for bodyIndex := 0; bodyIndex < callsAlg.Bodies().Count(); bodyIndex++ {
			body, bodyOK := callsAlg.Bodies().At(bodyIndex)
			if !bodyOK {
				continue
			}
			if _, returnOK := NewBodyReturn(schema, callsAlg, application, body); returnOK {
				t.Fatalf("ordinary call admitted BodyReturn: application=%v body=%v", application, body)
			}
		}
	}
}

func TestBodyRulesRejectForeignAndMalformedOperands(t *testing.T) {
	schema := transformBodySchema(t)
	callsAlg, callsOK := calldomain.New(schema.Link())
	if !callsOK {
		t.Fatal("Call algebra")
	}
	applications := schema.Link().Project().Applications().Calls()
	application, applicationOK := applications.At(0)
	body, bodyOK := callsAlg.Bodies().At(0)
	if !applicationOK || !bodyOK {
		t.Fatal("body operand fixture")
	}
	if _, ok := NewBodyEntry(schema, callsAlg, application, calldomain.Body{}); ok {
		t.Fatal("malformed BodyEntry operand admitted")
	}
	if _, ok := NewBodyReturn(schema, callsAlg, application, calldomain.Body{}); ok {
		t.Fatal("malformed BodyReturn operand admitted")
	}
	foreignSchema := transformBodySchema(t)
	if _, ok := NewBodyEntry(foreignSchema, callsAlg, application, body); ok {
		t.Fatal("foreign Pack schema crossed BodyEntry owner fence")
	}
	if _, ok := NewBodyReturn(foreignSchema, callsAlg, application, body); ok {
		t.Fatal("foreign Pack schema crossed BodyReturn owner fence")
	}

	composition := engine.NewComposition()
	packs, packsOK := packowner.Declare(composition, transformKey(100), schema)
	calls, callsOwnerOK := callowner.Declare(composition, transformKey(101), callsAlg)
	entryRule, entryRuleOK := DeclareBodyEntry(composition, transformKey(102), transformKey(103), transformKey(104), packs, calls)
	returnRule, returnRuleOK := DeclareBodyReturn(composition, transformKey(105), transformKey(106), transformKey(107), packs, calls)
	if !packsOK || !callsOwnerOK || !entryRuleOK || !returnRuleOK || entryRule == nil || returnRule == nil || !composition.Seal() {
		t.Fatal("body fence rule declarations")
	}
	if _, ok := entryRule.check(engine.RuleDerivation[packdomain.Value, BodyEntry]{}); ok {
		t.Fatal("malformed BodyEntry derivation admitted")
	}
	if _, ok := returnRule.check(engine.RuleDerivation[packdomain.Value, BodyReturn]{}); ok {
		t.Fatal("malformed BodyReturn derivation admitted")
	}
}
