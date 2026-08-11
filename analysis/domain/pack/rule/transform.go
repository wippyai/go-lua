// Package rule owns Pack's small cross-occurrence transformation judgments.
//
// The package deliberately keeps the operand vocabulary finite and typed:
// every operand is issued from a sealed Pack Schema (and, for body rules, an
// existing Call Body capability).  No Program/Link rows are copied into an
// operand, no application×body table is retained, and no solver is created
// here.  The engine's one SourceAssembly/Solver remains the recurrence and
// lifecycle owner.
package rule

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// Scalarization is one exact Pack-to-Pack Lua scalar adjustment.  The input
// and output are existing Pack roots; offset is the already-sealed TableIndex
// selected by the caller's static/Program boundary.
type Scalarization struct {
	input  packdomain.Root
	output packdomain.Root
	offset packdomain.TableIndex
	id     keyspace.ContentID
}

func NewScalarization(schema *packdomain.Schema, input, output packdomain.Root, offset packdomain.TableIndex) (Scalarization, bool) {
	if schema == nil || !schema.PackOnly(input) || !schema.PackOnly(output) || !schema.OwnsTableIndex(offset) {
		return Scalarization{}, false
	}
	inID, inOK := schema.RootID(input)
	outID, outOK := schema.RootID(output)
	offsetValue, offsetOK := offset.Value()
	if !inOK || !outOK || !offsetOK {
		return Scalarization{}, false
	}
	id := operationID("wippy.analysis.pack.scalarization.v1\x00", []keyspace.ContentID{inID, outID}, []uint64{offsetValue})
	return Scalarization{input: input, output: output, offset: offset, id: id}, id.Available()
}

func (value Scalarization) Input() packdomain.Root  { return value.input }
func (value Scalarization) Output() packdomain.Root { return value.output }
func (value Scalarization) Offset() packdomain.TableIndex {
	return value.offset
}
func (value Scalarization) ContentID() keyspace.ContentID { return value.id }

// Splice is one finite ordered Values-list judgment.  Inputs are existing
// Pack roots in authored order. Final selects Lua's final-expression rule.
type Splice struct {
	inputs []packdomain.Root
	output packdomain.Root
	final  bool
	id     keyspace.ContentID
}

func NewSplice(schema *packdomain.Schema, inputs []packdomain.Root, output packdomain.Root, final bool) (Splice, bool) {
	if schema == nil || !schema.PackOnly(output) {
		return Splice{}, false
	}
	ids := make([]keyspace.ContentID, 0, len(inputs)+1)
	for _, input := range inputs {
		if !schema.PackOnly(input) {
			return Splice{}, false
		}
		id, ok := schema.RootID(input)
		if !ok {
			return Splice{}, false
		}
		ids = append(ids, id)
	}
	outID, ok := schema.RootID(output)
	if !ok {
		return Splice{}, false
	}
	ids = append(ids, outID)
	numbers := []uint64{0}
	if final {
		numbers[0] = 1
	}
	id := operationID("wippy.analysis.pack.splice.v1\x00", ids, numbers)
	return Splice{inputs: append([]packdomain.Root(nil), inputs...), output: output, final: final, id: id}, id.Available()
}

func (value Splice) InputCount() int { return len(value.inputs) }
func (value Splice) InputAt(index int) (packdomain.Root, bool) {
	if index < 0 || index >= len(value.inputs) {
		return packdomain.Root{}, false
	}
	return value.inputs[index], true
}
func (value Splice) Output() packdomain.Root       { return value.output }
func (value Splice) Final() bool                   { return value.final }
func (value Splice) ContentID() keyspace.ContentID { return value.id }

// Bind is the checked fixed-width Lua bind. The output relation is the one
// atomic Body/Bind root containing scalar Cell equations and one residual Pack
// equation. Width is retained only as an immutable descriptor scalar.
type Bind struct {
	input  packdomain.Root
	output packdomain.Root
	bind   packdomain.Bind
	width  int
	id     keyspace.ContentID
}

func NewBind(schema *packdomain.Schema, bind packdomain.Bind) (Bind, bool) {
	if schema == nil {
		return Bind{}, false
	}
	input, inputOK := bind.InputRoot()
	output, outputOK := bind.Root()
	if !inputOK || !outputOK || !schema.PackOnly(input) || bind.CellCount() < 0 {
		return Bind{}, false
	}
	inID, inOK := schema.RootID(input)
	outID, outOK := schema.RootID(output)
	_, termOK := bind.Term()
	if !inOK || !outOK || !termOK {
		return Bind{}, false
	}
	// The sealed Bind root IDs already commit to the authored term.  Keep the
	// operation identity rooted in those owner-issued IDs and the fixed width;
	// carrying a raw Program term here would create a second IR vocabulary.
	id := operationID("wippy.analysis.pack.bind.v1\x00", []keyspace.ContentID{inID, outID}, []uint64{uint64(bind.CellCount())})
	return Bind{input: input, output: output, bind: bind, width: bind.CellCount(), id: id}, id.Available()
}

func (value Bind) Input() packdomain.Root        { return value.input }
func (value Bind) Output() packdomain.Root       { return value.output }
func (value Bind) Width() int                    { return value.width }
func (value Bind) Descriptor() packdomain.Bind   { return value.bind }
func (value Bind) ContentID() keyspace.ContentID { return value.id }

// BodyEntry transports one known Call Body's actual Pack into the Body root.
// Call remains the target capability owner; Pack only projects the existing
// formal Cells and entry Pack port.
type BodyEntry struct {
	application linkproject.Application
	body        calldomain.Body
	callRoot    packdomain.Root
	packBody    packdomain.Body
	id          keyspace.ContentID
}

func NewBodyEntry(schema *packdomain.Schema, calls *calldomain.Algebra, application linkproject.Application, body calldomain.Body) (BodyEntry, bool) {
	if schema == nil || calls == nil || calls.Link() != schema.Link() || !body.Valid() {
		return BodyEntry{}, false
	}
	key, keyOK := calls.KeyForApplication(application)
	callRoot, callRootOK := schema.CallRoot(application)
	shard, bodyTerm, bodyOK := calls.ResolveBody(body)
	bodyDesc, bodyDescOK := schema.Body(shard, bodyTerm)
	bodyRoot, bodyRootOK := bodyDesc.Root()
	if !keyOK || !key.IsApplication() || !callRootOK || !bodyOK || !bodyDescOK || !bodyRootOK || !schema.PackOnly(callRoot) {
		return BodyEntry{}, false
	}
	boundary, boundaryOK := bodyBoundary(schema, application)
	_, _, actuals, actualsOK := schema.Link().Boundary().Calls().CallOperands(application)
	callShard, callTerm, callTermOK := schemaCallTerm(schema, application)
	actualShard, actualsTerm, actualsOriginOK := schema.Link().Boundary().Values().Origin(actuals)
	if !boundaryOK || !actualsOK || !actualsOriginOK || !callTermOK || boundary.Call != callTerm || actualShard != callShard || actualsTerm == 0 {
		return BodyEntry{}, false
	}
	callID, callIDOK := key.ContentID()
	bodyID, bodyIDOK := body.ContentID()
	callRootID, callRootIDOK := schema.RootID(callRoot)
	bodyRootID, bodyRootIDOK := schema.RootID(bodyRoot)
	if !callIDOK || !bodyIDOK || !callRootIDOK || !bodyRootIDOK {
		return BodyEntry{}, false
	}
	id := operationID("wippy.analysis.pack.body-entry.v1\x00", []keyspace.ContentID{callID, bodyID, callRootID, bodyRootID}, nil)
	return BodyEntry{application: application, body: body, callRoot: callRoot, packBody: bodyDesc, id: id}, id.Available()
}

func (value BodyEntry) Application() linkproject.Application { return value.application }
func (value BodyEntry) Body() calldomain.Body                { return value.body }
func (value BodyEntry) CallRoot() packdomain.Root            { return value.callRoot }
func (value BodyEntry) ContentID() keyspace.ContentID        { return value.id }

// BodyReturn transports one Body Outcome Pack (canonical normal fallthrough or
// explicit Return) to the exact caller Call tail producer. plan is the causal caller OutcomeReturn
// descriptor; it remains distinct from outcome (the callee Body result) and
// is carried as an owner-fenced operand witness. It is intentionally separate
// from BodyEntry: each Rule owns one complete Pack output relation and WTO/SCC
// handles recursive re-entry.
type BodyReturn struct {
	application linkproject.Application
	body        calldomain.Body
	outcome     packdomain.Outcome
	plan        packdomain.Outcome
	callRoot    packdomain.Root
	id          keyspace.ContentID
}

func NewBodyReturn(schema *packdomain.Schema, calls *calldomain.Algebra, application linkproject.Application, body calldomain.Body) (BodyReturn, bool) {
	return newBodyReturn(schema, calls, application, body, true)
}

// NewBodyNormalReturn issues the sibling transport for canonical normal
// fallthrough. It deliberately uses Body.Normal's empty-Pack Outcome root;
// it is not an alias for explicit Return and never reuses that descriptor.
func NewBodyNormalReturn(schema *packdomain.Schema, calls *calldomain.Algebra, application linkproject.Application, body calldomain.Body) (BodyReturn, bool) {
	return newBodyReturn(schema, calls, application, body, false)
}

func newBodyReturn(schema *packdomain.Schema, calls *calldomain.Algebra, application linkproject.Application, body calldomain.Body, explicit bool) (BodyReturn, bool) {
	if schema == nil || calls == nil || calls.Link() != schema.Link() || !body.Valid() {
		return BodyReturn{}, false
	}
	key, keyOK := calls.KeyForApplication(application)
	shard, bodyTerm, bodyOK := calls.ResolveBody(body)
	bodyDesc, bodyDescOK := schema.Body(shard, bodyTerm)
	var outcome packdomain.Outcome
	var outcomeOK bool
	if explicit {
		outcome, outcomeOK = bodyDesc.Return()
	} else {
		outcome, outcomeOK = bodyDesc.Normal()
	}
	callShard, callTerm, callTermOK := schemaCallTerm(schema, application)
	if !keyOK || !key.IsApplication() || !bodyOK || !bodyDescOK || !outcomeOK || (explicit && outcome.Kind() != flowkind.OutcomeReturn) || (!explicit && outcome.Kind() != flowkind.OutcomeNormal) || !callTermOK {
		return BodyReturn{}, false
	}
	p, programOK := schema.Link().Project().Mounts().Program(callShard)
	if !programOK || p == nil {
		return BodyReturn{}, false
	}
	flowView := p.Flow()
	boundary, boundaryOK := flowView.Causal().Boundaries().For(callTerm)
	if !boundaryOK || boundary.Call != callTerm || boundary.TailReturn == 0 {
		return BodyReturn{}, false
	}
	plan, planOK := schema.Outcome(callShard, boundary.TailReturn)
	callerBody, _, _, _, callerCallOK := flowView.Authored().Calls().Get(callTerm)
	plannedBody, plannedBodyOK := plan.Body()
	if !planOK || plan.Kind() != flowkind.OutcomeReturn || !callerCallOK || !plannedBodyOK || plannedBody != callerBody {
		return BodyReturn{}, false
	}
	returns := flowView.Authored().Control().Returns()
	matchedReturn := false
	for index := 0; index < returns.Count(); index++ {
		returnTerm, returnOK := returns.At(index)
		owner, valuesTerm, rowOK := returns.Get(returnTerm)
		if !returnOK || !rowOK || owner != callerBody || !flowView.Executable().Contains(returnTerm) {
			continue
		}
		exit, exitOK := flowView.Outcomes().ReturnExit(returnTerm)
		if !exitOK || exit != boundary.TailReturn {
			continue
		}
		_, tail, valuesOK := flowView.Authored().Values().Get(valuesTerm)
		if !valuesOK || tail != callTerm {
			return BodyReturn{}, false
		}
		matchedReturn = true
	}
	if !matchedReturn {
		return BodyReturn{}, false
	}
	tailRoot, tailRootOK := schema.TailRoot(callShard, callTerm)
	tailProducer, tailProducerOK := schema.TailProducer(tailRoot)
	if !tailRootOK || !tailProducerOK || tailProducer.Kind() != packdomain.TailProducerCall || !schema.PackOnly(tailRoot) {
		return BodyReturn{}, false
	}
	callID, callIDOK := key.ContentID()
	bodyID, bodyIDOK := body.ContentID()
	outcomeRoot, _ := outcome.Root()
	planRoot, planRootOK := plan.Root()
	outcomeID, outcomeIDOK := schema.RootID(outcomeRoot)
	planID, planIDOK := schema.RootID(planRoot)
	tailID, tailIDOK := schema.RootID(tailRoot)
	if !callIDOK || !bodyIDOK || !outcomeIDOK || !planRootOK || !planIDOK || !tailIDOK {
		return BodyReturn{}, false
	}
	id := operationID("wippy.analysis.pack.body-return.v1\x00", []keyspace.ContentID{callID, bodyID, outcomeID, planID, tailID}, nil)
	return BodyReturn{application: application, body: body, outcome: outcome, plan: plan, callRoot: tailRoot, id: id}, id.Available()
}

func (value BodyReturn) Application() linkproject.Application { return value.application }
func (value BodyReturn) Body() calldomain.Body                { return value.body }
func (value BodyReturn) OutcomeRoot() packdomain.Root {
	root, _ := value.outcome.Root()
	return root
}
func (value BodyReturn) PlanRoot() packdomain.Root {
	root, _ := value.plan.Root()
	return root
}
func (value BodyReturn) CallRoot() packdomain.Root     { return value.callRoot }
func (value BodyReturn) ContentID() keyspace.ContentID { return value.id }

func operationID(prefix string, ids []keyspace.ContentID, numbers []uint64) keyspace.ContentID {
	h := sha256.New()
	_, _ = h.Write([]byte(prefix))
	for _, id := range ids {
		_, _ = h.Write(id[:])
	}
	for _, number := range numbers {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], number)
		_, _ = h.Write(encoded[:])
	}
	return keyspace.ContentID(sha256.Sum256(h.Sum(nil)))
}

func validDeclaration(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner) bool {
	return composition != nil && owner != nil && owner.Schema() != nil && semantic.Available() && family.Available() && evidence.Available() && semantic != family && semantic != evidence && family != evidence
}

func bodyBoundary(schema *packdomain.Schema, application linkproject.Application) (boundaryRow, bool) {
	if schema == nil || schema.Link() == nil || schema.Link().Project() == nil {
		return boundaryRow{}, false
	}
	shard, call, ok := schemaCallTerm(schema, application)
	if !ok {
		return boundaryRow{}, false
	}
	p, ok := schema.Link().Project().Mounts().Program(shard)
	if !ok || p == nil {
		return boundaryRow{}, false
	}
	boundary, ok := p.Flow().Causal().Boundaries().For(call)
	if !ok {
		return boundaryRow{}, false
	}
	if boundary.Call != call {
		return boundaryRow{}, false
	}
	return boundaryRow{Call: boundary.Call, Normal: boundary.Normal}, true
}

type boundaryRow struct {
	Call   keyspace.Term
	Normal keyspace.Term
}

func schemaCallTerm(schema *packdomain.Schema, application linkproject.Application) (linkproject.Shard, keyspace.Term, bool) {
	if schema == nil || schema.Link() == nil || schema.Link().Project() == nil {
		return linkproject.Shard{}, 0, false
	}
	return schema.Link().Project().Applications().Call(application)
}

func callValueHasBody(value calldomain.Value, body calldomain.Body) bool {
	if !body.Valid() || value.IsTop() {
		return false
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, ok := value.KnownTargetAt(index)
		if !ok {
			return false
		}
		candidate, bodyOK := target.Body()
		if bodyOK && candidate.Same(body) {
			return true
		}
	}
	return false
}

func mapScalar(schema *packdomain.Schema, input, output packdomain.Root, fact packdomain.Value, offset packdomain.TableIndex) (packdomain.Value, bool) {
	if schema == nil || fact.IsBottom() {
		return schema.Bottom(), schema != nil
	}
	if fact.IsTop() {
		return schema.Top(), true
	}
	terms, ok := schema.Terms(input, fact)
	if !ok {
		return packdomain.Value{}, false
	}
	builder, ok := schema.Builder(output)
	if !ok {
		return packdomain.Value{}, false
	}
	result := schema.Bottom()
	for _, source := range terms {
		scalars, scalarOK := builder.ScalarAlternatives(source, offset)
		if !scalarOK || len(scalars) == 0 {
			return packdomain.Value{}, false
		}
		for _, scalar := range scalars {
			term, termOK := builder.Closed(scalar)
			value, valueOK := builder.PackTerm(term)
			if !termOK || !valueOK {
				return packdomain.Value{}, false
			}
			result = schema.Lattice().Join(result, value)
		}
	}
	return result, true
}

func mapSplice(schema *packdomain.Schema, operand Splice, facts []packdomain.Value) (packdomain.Value, bool) {
	if schema == nil || len(facts) != operand.InputCount() {
		return packdomain.Value{}, false
	}
	builder, ok := schema.Builder(operand.output)
	if !ok {
		return packdomain.Value{}, false
	}
	terms := make([][]packdomain.Term, len(facts))
	for index, fact := range facts {
		if fact.IsTop() {
			return schema.Top(), true
		}
		values, valuesOK := schema.Terms(operand.inputs[index], fact)
		if !valuesOK {
			return packdomain.Value{}, false
		}
		if len(values) == 0 {
			return schema.Bottom(), true
		}
		terms[index] = values
	}
	result := schema.Bottom()
	chosen := make([]packdomain.Term, len(terms))
	var walk func(int) bool
	walk = func(index int) bool {
		if index == len(terms) {
			term, termOK := builder.Splice(chosen, operand.final)
			value, valueOK := builder.PackTerm(term)
			if !termOK || !valueOK {
				return false
			}
			result = schema.Lattice().Join(result, value)
			return true
		}
		for _, term := range terms[index] {
			chosen[index] = term
			if !walk(index + 1) {
				return false
			}
		}
		return true
	}
	if !walk(0) {
		return packdomain.Value{}, false
	}
	return result, true
}

// OutcomeSeed is the production normal-fallthrough seed. It has no Flow
// Values input: a normal Body exit contributes exactly the closed empty Pack.
// Explicit Return transport is a separate transformation below, because its
// causal Values vector is a variable-arity owner summary read.
type OutcomeSeed struct {
	outcome packdomain.Outcome
	id      keyspace.ContentID
}

func NewOutcomeSeed(schema *packdomain.Schema, outcome packdomain.Outcome) (OutcomeSeed, bool) {
	if schema == nil || !outcome.Same(outcome) || outcome.Kind() != flowkind.OutcomeNormal || outcome.ValuesCount() != 0 {
		return OutcomeSeed{}, false
	}
	root, rootOK := outcome.Root()
	if !rootOK || !schema.PackOnly(root) {
		return OutcomeSeed{}, false
	}
	rootID, idOK := schema.RootID(root)
	if !idOK {
		return OutcomeSeed{}, false
	}
	id := operationID("wippy.analysis.pack.outcome-seed.v1\x00", []keyspace.ContentID{rootID}, nil)
	return OutcomeSeed{outcome: outcome, id: id}, id.Available()
}

func (value OutcomeSeed) OutcomeRoot() packdomain.Root {
	root, _ := value.outcome.Root()
	return root
}
func (value OutcomeSeed) ContentID() keyspace.ContentID { return value.id }

func validOutcomeSeed(schema *packdomain.Schema, operand OutcomeSeed) bool {
	if schema == nil || !operand.outcome.Same(operand.outcome) || operand.outcome.Kind() != flowkind.OutcomeNormal || operand.outcome.ValuesCount() != 0 {
		return false
	}
	root, rootOK := operand.outcome.Root()
	if !rootOK || !schema.PackOnly(root) {
		return false
	}
	rootID, idOK := schema.RootID(root)
	expected := operationID("wippy.analysis.pack.outcome-seed.v1\x00", []keyspace.ContentID{rootID}, nil)
	return idOK && expected.Available() && expected == operand.id
}

func mapOutcomeSeed(schema *packdomain.Schema, operand OutcomeSeed) (packdomain.Value, bool) {
	if !validOutcomeSeed(schema, operand) {
		return packdomain.Value{}, false
	}
	builder, builderOK := schema.Builder(operand.OutcomeRoot())
	if !builderOK {
		return packdomain.Value{}, false
	}
	term, termOK := builder.Closed()
	if !termOK {
		return packdomain.Value{}, false
	}
	return builder.PackTerm(term)
}

type OutcomeSeedRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[packdomain.Value, OutcomeSeed]
	owner    *packowner.Owner
	write    engine.Write[packdomain.Value]
}

func DeclareOutcomeSeed(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner) (*OutcomeSeedRule, bool) {
	if !validDeclaration(composition, semantic, family, evidence, owner) {
		return nil, false
	}
	declaration := &OutcomeSeedRule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, OutcomeSeed]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.operandContent,
		Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:  declaration.transfer,
	}, func(rule *engine.Rule[packdomain.Value, OutcomeSeed]) bool {
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !writeOK {
			return false
		}
		declaration.rule, declaration.write = rule, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *OutcomeSeedRule) transfer(access engine.Access[packdomain.Value, OutcomeSeed]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || rule == nil || rule.owner == nil || !validOutcomeSeed(rule.owner.Schema(), operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		result, resultOK := mapOutcomeSeed(rule.owner.Schema(), operand)
		if !resultOK || result.IsBottom() {
			return false
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *OutcomeSeedRule) Instance(operand OutcomeSeed) (*engine.RuleInstance[packdomain.Value, OutcomeSeed], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !validOutcomeSeed(rule.owner.Schema(), operand) {
		return nil, false
	}
	outputRef, outputOK := rule.owner.Locate(operand.OutcomeRoot())
	if !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, OutcomeSeed]) bool {
		return engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func (rule *OutcomeSeedRule) validOperand(operand OutcomeSeed) bool {
	return rule != nil && rule.owner != nil && validOutcomeSeed(rule.owner.Schema(), operand)
}
func (rule *OutcomeSeedRule) operandContent(operand OutcomeSeed) (OutcomeSeed, [32]byte, bool) {
	if !rule.validOperand(operand) {
		return OutcomeSeed{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.id), true
}
func (rule *OutcomeSeedRule) check(derivation engine.RuleDerivation[packdomain.Value, OutcomeSeed]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	disposition, dispositionOK := derivation.DispositionAt(0)
	outputRef, outputOK := rule.owner.Locate(operand.OutcomeRoot())
	actual, actualOK := disposition.Value()
	expected, expectedOK := mapOutcomeSeed(rule.owner.Schema(), operand)
	target, targetOK := disposition.TargetAt(0)
	if !operandOK || !rule.validOperand(operand) || !derivation.OperandContentMatches([32]byte(operand.id)) || !dispositionOK || disposition.Guard().Empty() || !expectedOK || expected.IsBottom() || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK || !outputOK || !engine.TargetMatchesRef(target, outputRef) || !rule.owner.Schema().Lattice().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

// OutcomeTransfer transports an explicit Return's exact Flow Values
// alternatives. Each Values root is already one complete ordered Pack list;
// alternatives are unioned independently. Ordered expression-position
// Cartesian products belong to Splice within one Values root, never between
// authored Return alternatives. Its engine Rule has one fixed predecessor
// input, while the variable number of Values roots is bound per instance
// through Pack's sole SummaryRead/ClosedRefs path.
type OutcomeTransfer struct {
	outcome      packdomain.Outcome
	inputs       []packdomain.Root
	summaryOrder []int
	id           keyspace.ContentID
}

func NewOutcomeTransfer(schema *packdomain.Schema, outcome packdomain.Outcome) (OutcomeTransfer, bool) {
	if schema == nil || !outcome.Same(outcome) || outcome.Kind() != flowkind.OutcomeReturn || outcome.ValuesCount() == 0 {
		return OutcomeTransfer{}, false
	}
	outcomeRoot, outcomeRootOK := outcome.Root()
	if !outcomeRootOK || !schema.PackOnly(outcomeRoot) {
		return OutcomeTransfer{}, false
	}
	inputs := make([]packdomain.Root, 0, outcome.ValuesCount())
	ids := make([]keyspace.ContentID, 0, outcome.ValuesCount()+1)
	outcomeID, outcomeIDOK := schema.RootID(outcomeRoot)
	if !outcomeIDOK {
		return OutcomeTransfer{}, false
	}
	ids = append(ids, outcomeID)
	for index := 0; index < outcome.ValuesCount(); index++ {
		root, rootOK := outcome.ValuesRootAt(index)
		rootID, rootIDOK := schema.RootID(root)
		if !rootOK || !rootIDOK || !schema.PackOnly(root) {
			return OutcomeTransfer{}, false
		}
		inputs = append(inputs, root)
		ids = append(ids, rootID)
	}
	summaryOrder := make([]int, len(inputs))
	for index := range summaryOrder {
		summaryOrder[index] = index
	}
	sort.SliceStable(summaryOrder, func(left, right int) bool {
		leftOrder, leftOK := schema.RootOrder(inputs[summaryOrder[left]])
		rightOrder, rightOK := schema.RootOrder(inputs[summaryOrder[right]])
		return leftOK && rightOK && leftOrder < rightOrder
	})
	previousOrder := -1
	for _, inputIndex := range summaryOrder {
		rootOrder, rootOrderOK := schema.RootOrder(inputs[inputIndex])
		if !rootOrderOK || rootOrder <= previousOrder {
			return OutcomeTransfer{}, false
		}
		previousOrder = rootOrder
	}
	id := operationID("wippy.analysis.pack.outcome-transfer.v1\x00", ids, nil)
	return OutcomeTransfer{outcome: outcome, inputs: inputs, summaryOrder: summaryOrder, id: id}, id.Available()
}

func (value OutcomeTransfer) OutcomeRoot() packdomain.Root {
	root, _ := value.outcome.Root()
	return root
}
func (value OutcomeTransfer) InputCount() int { return len(value.inputs) }
func (value OutcomeTransfer) InputAt(index int) (packdomain.Root, bool) {
	if index < 0 || index >= len(value.inputs) {
		return packdomain.Root{}, false
	}
	return value.inputs[index], true
}
func (value OutcomeTransfer) ContentID() keyspace.ContentID { return value.id }

func outcomeTransferID(schema *packdomain.Schema, operand OutcomeTransfer) (keyspace.ContentID, bool) {
	if schema == nil || !schema.PackOnly(operand.OutcomeRoot()) || len(operand.inputs) == 0 {
		return keyspace.ContentID{}, false
	}
	ids := make([]keyspace.ContentID, 0, len(operand.inputs)+1)
	outcomeID, outcomeIDOK := schema.RootID(operand.OutcomeRoot())
	if !outcomeIDOK {
		return keyspace.ContentID{}, false
	}
	ids = append(ids, outcomeID)
	for _, input := range operand.inputs {
		if !schema.PackOnly(input) {
			return keyspace.ContentID{}, false
		}
		id, idOK := schema.RootID(input)
		if !idOK {
			return keyspace.ContentID{}, false
		}
		ids = append(ids, id)
	}
	return operationID("wippy.analysis.pack.outcome-transfer.v1\x00", ids, nil), true
}

func validOutcomeTransfer(schema *packdomain.Schema, operand OutcomeTransfer) bool {
	if schema == nil || !operand.outcome.Same(operand.outcome) || operand.outcome.Kind() != flowkind.OutcomeReturn {
		return false
	}
	outcomeRoot, outcomeRootOK := operand.outcome.Root()
	if !outcomeRootOK || !schema.PackOnly(outcomeRoot) || operand.InputCount() == 0 || operand.InputCount() != operand.outcome.ValuesCount() {
		return false
	}
	for index, input := range operand.inputs {
		expected, expectedOK := operand.outcome.ValuesRootAt(index)
		if !expectedOK || expected != input || !schema.PackOnly(input) {
			return false
		}
	}
	expectedID, expectedIDOK := outcomeTransferID(schema, operand)
	if !expectedIDOK || expectedID != operand.id || len(operand.summaryOrder) != len(operand.inputs) {
		return false
	}
	previousOrder := -1
	for _, inputIndex := range operand.summaryOrder {
		if inputIndex < 0 || inputIndex >= len(operand.inputs) {
			return false
		}
		rootOrder, rootOrderOK := schema.RootOrder(operand.inputs[inputIndex])
		if !rootOrderOK || rootOrder <= previousOrder {
			return false
		}
		previousOrder = rootOrder
	}
	return true
}

func mapOutcomeTransfer(schema *packdomain.Schema, operand OutcomeTransfer, facts []packdomain.Value) (packdomain.Value, bool) {
	if schema == nil || !validOutcomeTransfer(schema, operand) || len(facts) != len(operand.inputs) {
		return packdomain.Value{}, false
	}
	builder, builderOK := schema.Builder(operand.OutcomeRoot())
	if !builderOK {
		return packdomain.Value{}, false
	}
	orderedFacts := make([]packdomain.Value, len(facts))
	for summaryIndex, inputIndex := range operand.summaryOrder {
		if summaryIndex >= len(facts) || inputIndex < 0 || inputIndex >= len(orderedFacts) {
			return packdomain.Value{}, false
		}
		orderedFacts[inputIndex] = facts[summaryIndex]
	}
	result := schema.Bottom()
	for index, fact := range orderedFacts {
		if fact.IsBottom() {
			continue
		}
		if fact.IsTop() {
			return schema.Top(), true
		}
		terms, termsOK := schema.Terms(operand.inputs[index], fact)
		if !termsOK {
			return packdomain.Value{}, false
		}
		for _, term := range terms {
			value, valueOK := builder.PackTerm(term)
			if !valueOK {
				return packdomain.Value{}, false
			}
			result = schema.Lattice().Join(result, value)
		}
	}
	return result, true
}

type OutcomeTransferRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[packdomain.Value, OutcomeTransfer]
	owner    *packowner.Owner
	summary  engine.Read[engine.OrderedCells[packdomain.Value]]
	write    engine.Write[packdomain.Value]
}

func DeclareOutcomeTransfer(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner) (*OutcomeTransferRule, bool) {
	if !validDeclaration(composition, semantic, family, evidence, owner) {
		return nil, false
	}
	declaration := &OutcomeTransferRule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, OutcomeTransfer]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.operandContent,
		Output: owner.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:  declaration.transfer,
	}, func(rule *engine.Rule[packdomain.Value, OutcomeTransfer]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.SummaryRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !inputOK || !readOK || !writeOK {
			return false
		}
		declaration.rule, declaration.summary, declaration.write = rule, read, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *OutcomeTransferRule) transfer(access engine.Access[packdomain.Value, OutcomeTransfer]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || rule == nil || rule.owner == nil || !validOutcomeTransfer(rule.owner.Schema(), operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, cellsOK := engine.ReadValue(access, row, rule.summary)
		if !cellsOK || cells.Count() != operand.InputCount() {
			return false
		}
		facts := make([]packdomain.Value, operand.InputCount())
		for index := range facts {
			fact, present, available := cells.At(index)
			if !available {
				return false
			}
			if !present {
				facts[index] = rule.owner.Schema().Bottom()
				continue
			}
			facts[index] = fact
		}
		result, resultOK := mapOutcomeTransfer(rule.owner.Schema(), operand, facts)
		if !resultOK || result.IsBottom() {
			if resultOK {
				return engine.NoCandidate(access, row)
			}
			return false
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *OutcomeTransferRule) Instance(operand OutcomeTransfer) (*engine.RuleInstance[packdomain.Value, OutcomeTransfer], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !validOutcomeTransfer(rule.owner.Schema(), operand) {
		return nil, false
	}
	refs := outcomeSummaryRefs(rule.owner, operand)
	outputRef, outputOK := rule.owner.Locate(operand.OutcomeRoot())
	if refs == nil || !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, OutcomeTransfer]) bool {
		return packowner.InstanceSummaryRead(rule.owner, binding, rule.summary, refs) && engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func (rule *OutcomeTransferRule) validOperand(operand OutcomeTransfer) bool {
	return rule != nil && rule.owner != nil && validOutcomeTransfer(rule.owner.Schema(), operand)
}

func (rule *OutcomeTransferRule) operandContent(operand OutcomeTransfer) (OutcomeTransfer, [32]byte, bool) {
	if !rule.validOperand(operand) {
		return OutcomeTransfer{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.id), true
}

func (rule *OutcomeTransferRule) check(derivation engine.RuleDerivation[packdomain.Value, OutcomeTransfer]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !rule.validOperand(operand) || !derivation.OperandContentMatches([32]byte(operand.id)) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !dispositionOK || disposition.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	refs := outcomeSummaryRefs(rule.owner, operand)
	if !inputOK || refs == nil || !input.Guard().Same(disposition.Guard()) || !packowner.DerivationReadMatchesSummaryRefs(rule.owner, derivation, rule.summary, refs) {
		return engine.RuleEvidence{}, false
	}
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.summary)
	if !cellsOK || cells.Count() != operand.InputCount() {
		return engine.RuleEvidence{}, false
	}
	facts := make([]packdomain.Value, operand.InputCount())
	for index := range facts {
		fact, present, available := cells.At(index)
		if !available {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}
		if !present {
			facts[index] = rule.owner.Schema().Bottom()
			continue
		}
		facts[index] = fact
	}
	expected, expectedOK := mapOutcomeTransfer(rule.owner.Schema(), operand, facts)
	if !expectedOK {
		return engine.RuleEvidence{}, false
	}
	if expected.IsBottom() {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	outputRef, outputOK := rule.owner.Locate(operand.OutcomeRoot())
	if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK || !outputOK || !engine.TargetMatchesRef(target, outputRef) || !rule.owner.Schema().Lattice().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

func outcomeSummaryRefs(owner *packowner.Owner, operand OutcomeTransfer) *packowner.SummaryRefs {
	if owner == nil || !validOutcomeTransfer(owner.Schema(), operand) {
		return nil
	}
	refs := owner.NewSummaryRefs()
	if refs == nil {
		return nil
	}
	for _, inputIndex := range operand.summaryOrder {
		if inputIndex < 0 || inputIndex >= len(operand.inputs) || !owner.AppendSummaryRoot(refs, operand.inputs[inputIndex]) {
			return nil
		}
	}
	if !owner.CloseSummaryRefs(refs) {
		return nil
	}
	return refs
}

func mapBind(schema *packdomain.Schema, operand Bind, fact packdomain.Value) (packdomain.Value, bool) {
	if schema == nil || fact.IsBottom() {
		return schema.Bottom(), schema != nil
	}
	if fact.IsTop() {
		return schema.Top(), true
	}
	terms, ok := schema.Terms(operand.input, fact)
	if !ok {
		return packdomain.Value{}, false
	}
	builder, ok := schema.Builder(operand.output)
	if !ok {
		return packdomain.Value{}, false
	}
	descriptor := operand.bind
	result := schema.Bottom()
	for _, source := range terms {
		branches, bindOK := builder.BindAlternatives(source, operand.width)
		if !bindOK {
			return packdomain.Value{}, false
		}
		for _, branch := range branches {
			residual, residualOK := branch.Residual()
			equations := make([]packdomain.Equation, 0, operand.width+1)
			for index := 0; index < operand.width; index++ {
				cell, cellOK := descriptor.CellAt(index)
				scalar, scalarOK := branch.FixedAt(index)
				equation, equationOK := builder.Scalar(cell, scalar)
				if !cellOK || !residualOK || !scalarOK || !equationOK {
					return packdomain.Value{}, false
				}
				equations = append(equations, equation)
			}
			port, portOK := descriptor.Port()
			packEquation, packOK := builder.Pack(port, residual)
			if !portOK || !packOK {
				return packdomain.Value{}, false
			}
			equations = append(equations, packEquation)
			caseValue, caseOK := builder.Case(equations...)
			value, valueOK := builder.Value(caseValue)
			if !caseOK || !valueOK {
				return packdomain.Value{}, false
			}
			result = schema.Lattice().Join(result, value)
		}
	}
	return result, true
}

func mapBodyEntry(schema *packdomain.Schema, operand BodyEntry, fact packdomain.Value) (packdomain.Value, bool) {
	if schema == nil || fact.IsBottom() {
		return schema.Bottom(), schema != nil
	}
	if fact.IsTop() {
		return schema.Top(), true
	}
	terms, ok := schema.Terms(operand.callRoot, fact)
	if !ok {
		return packdomain.Value{}, false
	}
	bodyRoot, bodyRootOK := operand.packBody.Root()
	if !bodyRootOK {
		return packdomain.Value{}, false
	}
	builder, ok := schema.Builder(bodyRoot)
	if !ok {
		return packdomain.Value{}, false
	}
	bodyDesc := operand.packBody
	result := schema.Bottom()
	for _, source := range terms {
		branches, bindOK := builder.BindAlternatives(source, bodyDesc.FormalCount())
		if !bindOK {
			return packdomain.Value{}, false
		}
		for _, branch := range branches {
			residual, residualOK := branch.Residual()
			equations := make([]packdomain.Equation, 0, bodyDesc.FormalCount()+1)
			for index := 0; index < bodyDesc.FormalCount(); index++ {
				endpoint, endpointOK := bodyDesc.FormalAt(index)
				scalar, scalarOK := branch.FixedAt(index)
				equation, equationOK := builder.Scalar(endpoint, scalar)
				if !endpointOK || !residualOK || !scalarOK || !equationOK {
					return packdomain.Value{}, false
				}
				equations = append(equations, equation)
			}
			port, portOK := bodyDesc.Port()
			packEquation, packOK := builder.Pack(port, residual)
			if !portOK || !packOK {
				return packdomain.Value{}, false
			}
			equations = append(equations, packEquation)
			caseValue, caseOK := builder.Case(equations...)
			value, valueOK := builder.Value(caseValue)
			if !caseOK || !valueOK {
				return packdomain.Value{}, false
			}
			result = schema.Lattice().Join(result, value)
		}
	}
	return result, true
}

func mapBodyReturn(schema *packdomain.Schema, operand BodyReturn, fact packdomain.Value) (packdomain.Value, bool) {
	if schema == nil || fact.IsBottom() {
		return schema.Bottom(), schema != nil
	}
	if fact.IsTop() {
		return schema.Top(), true
	}
	outcomeRoot := operand.OutcomeRoot()
	terms, ok := schema.Terms(outcomeRoot, fact)
	if !ok {
		return packdomain.Value{}, false
	}
	builder, ok := schema.Builder(operand.callRoot)
	if !ok {
		return packdomain.Value{}, false
	}
	if operand.outcome.Kind() == flowkind.OutcomeNormal {
		if operand.outcome.ValuesCount() != 0 {
			return packdomain.Value{}, false
		}
		empty, emptyOK := builder.Closed()
		if !emptyOK || len(terms) == 0 {
			return packdomain.Value{}, false
		}
		// Normal completion has one canonical empty-Pack term.  Do not let a
		// malformed injected fact turn an arbitrary normal Outcome term into
		// the caller's empty result; the production OutcomeSeed is the sole
		// issuer of this path.
		for _, term := range terms {
			if !term.Equal(empty) {
				return packdomain.Value{}, false
			}
		}
		return builder.PackTerm(empty)
	}
	if operand.outcome.Kind() != flowkind.OutcomeReturn {
		return packdomain.Value{}, false
	}
	result := schema.Bottom()
	for _, term := range terms {
		value, valueOK := builder.PackTerm(term)
		if !valueOK {
			return packdomain.Value{}, false
		}
		result = schema.Lattice().Join(result, value)
	}
	return result, true
}

// ScalarizationRule is Pack's one-input Lua adjustment Rule. Its transfer
// writes a complete output Pack relation for every input alternative; a
// scalar marginal is never emitted as a standalone Factor value.
type ScalarizationRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[packdomain.Value, Scalarization]
	owner    *packowner.Owner
	read     engine.Read[engine.OrderedCells[packdomain.Value]]
	write    engine.Write[packdomain.Value]
}

func DeclareScalarization(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner) (*ScalarizationRule, bool) {
	if !validDeclaration(composition, semantic, family, evidence, owner) {
		return nil, false
	}
	declaration := &ScalarizationRule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, Scalarization]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.operandContent,
		Output: owner.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:  declaration.transfer,
	}, func(rule *engine.Rule[packdomain.Value, Scalarization]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.ExactRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !inputOK || !readOK || !writeOK {
			return false
		}
		declaration.rule, declaration.read, declaration.write = rule, read, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *ScalarizationRule) transfer(access engine.Access[packdomain.Value, Scalarization]) bool {
	operand, ok := engine.Operand(access)
	if !ok || rule == nil || !rule.validOperand(operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, rule.read)
		if !readOK || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present || fact.IsBottom() {
			return engine.NoCandidate(access, row)
		}
		result, resultOK := mapScalar(rule.owner.Schema(), operand.input, operand.output, fact, operand.offset)
		if !resultOK {
			return false
		}
		if result.IsBottom() {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *ScalarizationRule) Instance(operand Scalarization) (*engine.RuleInstance[packdomain.Value, Scalarization], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !rule.validOperand(operand) {
		return nil, false
	}
	inputRef, inputOK := rule.owner.Locate(operand.input)
	outputRef, outputOK := rule.owner.Locate(operand.output)
	if !inputOK || !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, Scalarization]) bool {
		return engine.InstanceRead(binding, rule.read, inputRef) && engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func (rule *ScalarizationRule) validOperand(operand Scalarization) bool {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return false
	}
	schema := rule.owner.Schema()
	if !schema.PackOnly(operand.input) || !schema.PackOnly(operand.output) || !schema.OwnsTableIndex(operand.offset) {
		return false
	}
	inID, inOK := schema.RootID(operand.input)
	outID, outOK := schema.RootID(operand.output)
	offsetValue, offsetOK := operand.offset.Value()
	if !inOK || !outOK || !offsetOK {
		return false
	}
	expected := operationID("wippy.analysis.pack.scalarization.v1\x00", []keyspace.ContentID{inID, outID}, []uint64{offsetValue})
	return expected.Available() && expected == operand.id
}

func coldScalarizationOperand(schema *packdomain.Schema, operand Scalarization) bool {
	expected, ok := NewScalarization(schema, operand.input, operand.output, operand.offset)
	return ok && expected == operand
}

func (rule *ScalarizationRule) operandContent(operand Scalarization) (Scalarization, [32]byte, bool) {
	if !rule.validOperand(operand) {
		return Scalarization{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.id), true
}

func (rule *ScalarizationRule) check(derivation engine.RuleDerivation[packdomain.Value, Scalarization]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !coldScalarizationOperand(rule.owner.Schema(), operand) || !derivation.OperandContentMatches([32]byte(operand.id)) {
		return engine.RuleEvidence{}, false
	}
	inputRef, inputOK := rule.owner.Locate(operand.input)
	outputRef, outputOK := rule.owner.Locate(operand.output)
	input, inputRowOK := derivation.InputAt(0)
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !inputOK || !outputOK || !inputRowOK || !dispositionOK || disposition.Guard().Empty() || !input.Guard().Same(disposition.Guard()) || !engine.DerivationReadMatchesRef(derivation, rule.read, inputRef) {
		return engine.RuleEvidence{}, false
	}
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
	if !cellsOK || cells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	fact, present, available := cells.At(0)
	if !available {
		return engine.RuleEvidence{}, false
	}
	if !present || fact.IsBottom() {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	expected, expectedOK := mapScalar(rule.owner.Schema(), operand.input, operand.output, fact, operand.offset)
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if !expectedOK || expected.IsBottom() || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK || !engine.TargetMatchesRef(target, outputRef) || !rule.owner.Schema().Lattice().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

// SpliceRule is the ordered multi-input Pack judgment. Each input read is a
// complete Pack factor and the output is one complete Pack factor.
type SpliceRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[packdomain.Value, Splice]
	owner    *packowner.Owner
	reads    []engine.Read[engine.OrderedCells[packdomain.Value]]
	write    engine.Write[packdomain.Value]
}

func DeclareSplice(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner, inputs int) (*SpliceRule, bool) {
	if !validDeclaration(composition, semantic, family, evidence, owner) || inputs < 0 {
		return nil, false
	}
	declaration := &SpliceRule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, Splice]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.operandContent,
		Output: owner.Output(), Inputs: inputs,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:  declaration.transfer,
	}, func(rule *engine.Rule[packdomain.Value, Splice]) bool {
		declaration.reads = make([]engine.Read[engine.OrderedCells[packdomain.Value]], inputs)
		for index := 0; index < inputs; index++ {
			input, inputOK := rule.InputAt(index)
			read, readOK := engine.ReadFrom(rule, input, owner.ExactRead())
			if !inputOK || !readOK {
				return false
			}
			declaration.reads[index] = read
		}
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !writeOK {
			return false
		}
		declaration.rule, declaration.write = rule, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *SpliceRule) transfer(access engine.Access[packdomain.Value, Splice]) bool {
	operand, ok := engine.Operand(access)
	if !ok || rule == nil || !rule.validOperand(operand) || len(rule.reads) != operand.InputCount() {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		facts := make([]packdomain.Value, len(rule.reads))
		for index, read := range rule.reads {
			cells, readOK := engine.ReadValue(access, row, read)
			if !readOK || cells.Count() != 1 {
				return false
			}
			fact, present, available := cells.At(0)
			if !available {
				return false
			}
			if !present || fact.IsBottom() {
				return engine.NoCandidate(access, row)
			}
			facts[index] = fact
		}
		result, resultOK := mapSplice(rule.owner.Schema(), operand, facts)
		if !resultOK {
			return false
		}
		if result.IsBottom() {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *SpliceRule) Instance(operand Splice) (*engine.RuleInstance[packdomain.Value, Splice], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !rule.validOperand(operand) || len(rule.reads) != operand.InputCount() {
		return nil, false
	}
	outputRef, outputOK := rule.owner.Locate(operand.output)
	if !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, Splice]) bool {
		for index, read := range rule.reads {
			root, rootOK := operand.InputAt(index)
			ref, refOK := rule.owner.Locate(root)
			if !rootOK || !refOK || !engine.InstanceRead(binding, read, ref) {
				return false
			}
		}
		return engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func (rule *SpliceRule) validOperand(operand Splice) bool {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return false
	}
	expected, ok := NewSplice(rule.owner.Schema(), operand.inputs, operand.output, operand.final)
	if !ok || expected.id != operand.id || expected.output != operand.output || expected.final != operand.final || len(expected.inputs) != len(operand.inputs) {
		return false
	}
	for index := range expected.inputs {
		if expected.inputs[index] != operand.inputs[index] {
			return false
		}
	}
	return true
}

func (rule *SpliceRule) operandContent(operand Splice) (Splice, [32]byte, bool) {
	if !rule.validOperand(operand) {
		return Splice{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.id), true
}

func (rule *SpliceRule) check(derivation engine.RuleDerivation[packdomain.Value, Splice]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != len(rule.reads) || derivation.ReadCount() != len(rule.reads) || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !rule.validOperand(operand) || !derivation.OperandContentMatches([32]byte(operand.id)) {
		return engine.RuleEvidence{}, false
	}
	facts := make([]packdomain.Value, len(rule.reads))
	inputRefsOK := true
	inputRowsOK := true
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !dispositionOK || disposition.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	for index, read := range rule.reads {
		input, inputOK := derivation.InputAt(index)
		root, rootOK := operand.InputAt(index)
		ref, refOK := rule.owner.Locate(root)
		inputRowsOK = inputRowsOK && inputOK && rootOK && input.Guard().Same(disposition.Guard())
		inputRefsOK = inputRefsOK && refOK && engine.DerivationReadMatchesRef(derivation, read, ref)
		cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, read)
		if !cellsOK || cells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		fact, present, available := cells.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present || fact.IsBottom() {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			if !inputRefsOK || !inputRowsOK {
				return engine.RuleEvidence{}, false
			}
			return derivation.Accept()
		}
		facts[index] = fact
	}
	if !inputRefsOK || !inputRowsOK {
		return engine.RuleEvidence{}, false
	}
	expected, expectedOK := mapSplice(rule.owner.Schema(), operand, facts)
	outputRef, outputOK := rule.owner.Locate(operand.output)
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if !expectedOK || expected.IsBottom() || !outputOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK || !engine.TargetMatchesRef(target, outputRef) || !rule.owner.Schema().Lattice().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

// BindRule owns the atomic Pack→Cell/Pack equation.  The output relation is
// richer than a Pack-only root, so no intermediate fixed Pack is published.
type BindRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[packdomain.Value, Bind]
	owner    *packowner.Owner
	read     engine.Read[engine.OrderedCells[packdomain.Value]]
	write    engine.Write[packdomain.Value]
}

func DeclareBind(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *packowner.Owner) (*BindRule, bool) {
	if !validDeclaration(composition, semantic, family, evidence, owner) {
		return nil, false
	}
	declaration := &BindRule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, Bind]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.operandContent,
		Output: owner.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:  declaration.transfer,
	}, func(rule *engine.Rule[packdomain.Value, Bind]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.ExactRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !inputOK || !readOK || !writeOK {
			return false
		}
		declaration.rule, declaration.read, declaration.write = rule, read, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *BindRule) transfer(access engine.Access[packdomain.Value, Bind]) bool {
	operand, ok := engine.Operand(access)
	if !ok || rule == nil || !rule.validOperand(operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, rule.read)
		if !readOK || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present || fact.IsBottom() {
			return engine.NoCandidate(access, row)
		}
		result, resultOK := mapBind(rule.owner.Schema(), operand, fact)
		if !resultOK {
			return false
		}
		if result.IsBottom() {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *BindRule) Instance(operand Bind) (*engine.RuleInstance[packdomain.Value, Bind], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil || !rule.validOperand(operand) {
		return nil, false
	}
	inputRef, inputOK := rule.owner.Locate(operand.input)
	outputRef, outputOK := rule.owner.Locate(operand.output)
	if !inputOK || !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, Bind]) bool {
		return engine.InstanceRead(binding, rule.read, inputRef) && engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func (rule *BindRule) validOperand(operand Bind) bool {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return false
	}
	expected, ok := NewBind(rule.owner.Schema(), operand.bind)
	return ok && expected == operand
}

func (rule *BindRule) operandContent(operand Bind) (Bind, [32]byte, bool) {
	if !rule.validOperand(operand) {
		return Bind{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.id), true
}

func (rule *BindRule) check(derivation engine.RuleDerivation[packdomain.Value, Bind]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !rule.validOperand(operand) || !derivation.OperandContentMatches([32]byte(operand.id)) {
		return engine.RuleEvidence{}, false
	}
	inputRef, inputOK := rule.owner.Locate(operand.input)
	outputRef, outputOK := rule.owner.Locate(operand.output)
	input, inputRowOK := derivation.InputAt(0)
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !inputOK || !outputOK || !inputRowOK || !dispositionOK || disposition.Guard().Empty() || !input.Guard().Same(disposition.Guard()) || !engine.DerivationReadMatchesRef(derivation, rule.read, inputRef) {
		return engine.RuleEvidence{}, false
	}
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
	if !cellsOK || cells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	fact, present, available := cells.At(0)
	if !available {
		return engine.RuleEvidence{}, false
	}
	if !present || fact.IsBottom() {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	expected, expectedOK := mapBind(rule.owner.Schema(), operand, fact)
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if !expectedOK || expected.IsBottom() || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK || !engine.TargetMatchesRef(target, outputRef) || !rule.owner.Schema().Lattice().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

func validBodyDeclaration(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, packs *packowner.Owner, calls *callowner.Owner) bool {
	return validDeclaration(composition, semantic, family, evidence, packs) && calls != nil && calls.Algebra() != nil && calls.Link() != nil && packs.Schema().Link() == calls.Link()
}

// BodyEntryRule is the direct Call-body formal-entry transfer. It reads the
// Call capability and the exact caller Pack root, then writes the one Body
// relation containing formal scalar equations and the residual entry Pack.
type BodyEntryRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[packdomain.Value, BodyEntry]
	packs    *packowner.Owner
	calls    *callowner.Owner
	callRead engine.Read[engine.OrderedCells[calldomain.Value]]
	packRead engine.Read[engine.OrderedCells[packdomain.Value]]
	write    engine.Write[packdomain.Value]
}

func DeclareBodyEntry(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, packs *packowner.Owner, calls *callowner.Owner) (*BodyEntryRule, bool) {
	if !validBodyDeclaration(composition, semantic, family, evidence, packs, calls) {
		return nil, false
	}
	declaration := &BodyEntryRule{semantic: semantic, packs: packs, calls: calls}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, BodyEntry]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.operandContent,
		Output: packs.Output(), Inputs: 2,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:  declaration.transfer,
	}, func(rule *engine.Rule[packdomain.Value, BodyEntry]) bool {
		callInput, callInputOK := rule.InputAt(0)
		packInput, packInputOK := rule.InputAt(1)
		callRead, callReadOK := engine.ReadFrom(rule, callInput, calls.ExactRead())
		packRead, packReadOK := engine.ReadFrom(rule, packInput, packs.ExactRead())
		write, writeOK := engine.WriteTo(rule, packs.ExactWrite())
		if !callInputOK || !packInputOK || !callReadOK || !packReadOK || !writeOK {
			return false
		}
		declaration.rule, declaration.callRead, declaration.packRead, declaration.write = rule, callRead, packRead, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *BodyEntryRule) transfer(access engine.Access[packdomain.Value, BodyEntry]) bool {
	operand, ok := engine.Operand(access)
	if !ok || rule == nil || !rule.validOperand(operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		callCells, callReadOK := engine.ReadValue(access, row, rule.callRead)
		packCells, packReadOK := engine.ReadValue(access, row, rule.packRead)
		if !callReadOK || !packReadOK || callCells.Count() != 1 || packCells.Count() != 1 {
			return false
		}
		callFact, callPresent, callAvailable := callCells.At(0)
		packFact, packPresent, packAvailable := packCells.At(0)
		if !callAvailable || !packAvailable {
			return false
		}
		if !callPresent || !packPresent || callFact.IsEmpty() || packFact.IsBottom() || !callValueHasBody(callFact, operand.body) {
			return engine.NoCandidate(access, row)
		}
		result, resultOK := mapBodyEntry(rule.packs.Schema(), operand, packFact)
		if !resultOK || result.IsBottom() {
			if resultOK {
				return engine.NoCandidate(access, row)
			}
			return false
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *BodyEntryRule) Instance(operand BodyEntry) (*engine.RuleInstance[packdomain.Value, BodyEntry], bool) {
	if rule == nil || rule.rule == nil || !rule.validOperand(operand) {
		return nil, false
	}
	key, keyOK := rule.calls.Algebra().KeyForApplication(operand.application)
	callRef, callRefOK := rule.calls.Locate(key)
	packRef, packRefOK := rule.packs.Locate(operand.callRoot)
	bodyRoot, bodyRootOK := operand.packBody.Root()
	outputRef, outputRefOK := rule.packs.Locate(bodyRoot)
	if !keyOK || !callRefOK || !packRefOK || !bodyRootOK || !outputRefOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, BodyEntry]) bool {
		return engine.InstanceRead(binding, rule.callRead, callRef) && engine.InstanceRead(binding, rule.packRead, packRef) && engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func (rule *BodyEntryRule) validOperand(operand BodyEntry) bool {
	if rule == nil || rule.packs == nil || rule.calls == nil || rule.calls.Algebra() == nil || rule.packs.Schema() == nil || !operand.body.Valid() {
		return false
	}
	schema := rule.packs.Schema()
	key, keyOK := rule.calls.Algebra().KeyForApplication(operand.application)
	callRoot, callRootOK := schema.CallRoot(operand.application)
	bodyRoot, bodyRootOK := operand.packBody.Root()
	bodyRootID, bodyRootIDOK := schema.RootID(bodyRoot)
	callID, callIDOK := key.ContentID()
	bodyID, bodyIDOK := operand.body.ContentID()
	callRootID, callRootIDOK := schema.RootID(callRoot)
	if !keyOK || !key.IsApplication() || !callRootOK || !bodyRootOK || !callIDOK || !bodyIDOK || !callRootIDOK || !bodyRootIDOK || !schema.PackOnly(callRoot) || callRoot != operand.callRoot || !schema.PackOnly(bodyRoot) {
		return false
	}
	if !operand.packBody.Same(operand.packBody) {
		return false
	}
	expectedID := operationID("wippy.analysis.pack.body-entry.v1\x00", []keyspace.ContentID{callID, bodyID, callRootID, bodyRootID}, nil)
	return expectedID.Available() && expectedID == operand.id
}

func coldBodyEntryOperand(schema *packdomain.Schema, calls *calldomain.Algebra, operand BodyEntry) bool {
	expected, ok := NewBodyEntry(schema, calls, operand.application, operand.body)
	return ok && expected == operand
}

func (rule *BodyEntryRule) operandContent(operand BodyEntry) (BodyEntry, [32]byte, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil || !coldBodyEntryOperand(rule.packs.Schema(), rule.calls.Algebra(), operand) {
		return BodyEntry{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.id), true
}

func (rule *BodyEntryRule) check(derivation engine.RuleDerivation[packdomain.Value, BodyEntry]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 2 || derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !coldBodyEntryOperand(rule.packs.Schema(), rule.calls.Algebra(), operand) || !derivation.OperandContentMatches([32]byte(operand.id)) {
		return engine.RuleEvidence{}, false
	}
	key, keyOK := rule.calls.Algebra().KeyForApplication(operand.application)
	callRef, callRefOK := rule.calls.Locate(key)
	packRef, packRefOK := rule.packs.Locate(operand.callRoot)
	bodyRoot, bodyRootOK := operand.packBody.Root()
	outputRef, outputRefOK := rule.packs.Locate(bodyRoot)
	disposition, dispositionOK := derivation.DispositionAt(0)
	callInput, callInputOK := derivation.InputAt(0)
	packInput, packInputOK := derivation.InputAt(1)
	if !keyOK || !callRefOK || !packRefOK || !bodyRootOK || !outputRefOK || !dispositionOK || !callInputOK || !packInputOK || disposition.Guard().Empty() || !callInput.Guard().Same(disposition.Guard()) || !packInput.Guard().Same(disposition.Guard()) || !engine.DerivationReadMatchesRef(derivation, rule.callRead, callRef) || !engine.DerivationReadMatchesRef(derivation, rule.packRead, packRef) {
		return engine.RuleEvidence{}, false
	}
	callCells, callCellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.callRead)
	packCells, packCellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.packRead)
	if !callCellsOK || !packCellsOK || callCells.Count() != 1 || packCells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	callFact, callPresent, callAvailable := callCells.At(0)
	packFact, packPresent, packAvailable := packCells.At(0)
	if !callAvailable || !packAvailable {
		return engine.RuleEvidence{}, false
	}
	if !callPresent || !packPresent || callFact.IsEmpty() || packFact.IsBottom() || !callValueHasBody(callFact, operand.body) {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	expected, expectedOK := mapBodyEntry(rule.packs.Schema(), operand, packFact)
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if !expectedOK || expected.IsBottom() || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK || !engine.TargetMatchesRef(target, outputRef) || !rule.packs.Schema().Lattice().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

// BodyReturnRule is the direct normal-return transfer from one Body Outcome
// Pack to its caller's Call-tail producer. It consumes the existing Call
// Body capability to keep direct and opaque/open call arms separate.
type BodyReturnRule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[packdomain.Value, BodyReturn]
	packs    *packowner.Owner
	calls    *callowner.Owner
	callRead engine.Read[engine.OrderedCells[calldomain.Value]]
	packRead engine.Read[engine.OrderedCells[packdomain.Value]]
	write    engine.Write[packdomain.Value]
}

func DeclareBodyReturn(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, packs *packowner.Owner, calls *callowner.Owner) (*BodyReturnRule, bool) {
	if !validBodyDeclaration(composition, semantic, family, evidence, packs, calls) {
		return nil, false
	}
	declaration := &BodyReturnRule{semantic: semantic, packs: packs, calls: calls}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, BodyReturn]{
		Semantic: semantic, OperandFamily: family, OperandContent: declaration.operandContent,
		Output: packs.Output(), Inputs: 2,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:  declaration.transfer,
	}, func(rule *engine.Rule[packdomain.Value, BodyReturn]) bool {
		callInput, callInputOK := rule.InputAt(0)
		packInput, packInputOK := rule.InputAt(1)
		callRead, callReadOK := engine.ReadFrom(rule, callInput, calls.ExactRead())
		packRead, packReadOK := engine.ReadFrom(rule, packInput, packs.ExactRead())
		write, writeOK := engine.WriteTo(rule, packs.ExactWrite())
		if !callInputOK || !packInputOK || !callReadOK || !packReadOK || !writeOK {
			return false
		}
		declaration.rule, declaration.callRead, declaration.packRead, declaration.write = rule, callRead, packRead, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *BodyReturnRule) transfer(access engine.Access[packdomain.Value, BodyReturn]) bool {
	operand, ok := engine.Operand(access)
	if !ok || rule == nil || !rule.validOperand(operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		callCells, callReadOK := engine.ReadValue(access, row, rule.callRead)
		packCells, packReadOK := engine.ReadValue(access, row, rule.packRead)
		if !callReadOK || !packReadOK || callCells.Count() != 1 || packCells.Count() != 1 {
			return false
		}
		callFact, callPresent, callAvailable := callCells.At(0)
		packFact, packPresent, packAvailable := packCells.At(0)
		if !callAvailable || !packAvailable {
			return false
		}
		if !callPresent || !packPresent || callFact.IsEmpty() || packFact.IsBottom() || !callValueHasBody(callFact, operand.body) {
			return engine.NoCandidate(access, row)
		}
		result, resultOK := mapBodyReturn(rule.packs.Schema(), operand, packFact)
		if !resultOK || result.IsBottom() {
			if resultOK {
				return engine.NoCandidate(access, row)
			}
			return false
		}
		return engine.StageValue(access, row, result)
	})
}

func (rule *BodyReturnRule) Instance(operand BodyReturn) (*engine.RuleInstance[packdomain.Value, BodyReturn], bool) {
	if rule == nil || rule.rule == nil || !rule.validOperand(operand) {
		return nil, false
	}
	key, keyOK := rule.calls.Algebra().KeyForApplication(operand.application)
	callRef, callRefOK := rule.calls.Locate(key)
	outcomeRoot := operand.OutcomeRoot()
	packRef, packRefOK := rule.packs.Locate(outcomeRoot)
	outputRef, outputRefOK := rule.packs.Locate(operand.callRoot)
	if !keyOK || !callRefOK || !packRefOK || !outputRefOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[packdomain.Value, BodyReturn]) bool {
		return engine.InstanceRead(binding, rule.callRead, callRef) && engine.InstanceRead(binding, rule.packRead, packRef) && engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func (rule *BodyReturnRule) validOperand(operand BodyReturn) bool {
	if rule == nil || rule.packs == nil || rule.calls == nil || rule.calls.Algebra() == nil || rule.packs.Schema() == nil || !operand.body.Valid() {
		return false
	}
	schema := rule.packs.Schema()
	key, keyOK := rule.calls.Algebra().KeyForApplication(operand.application)
	planRoot, planRootOK := operand.plan.Root()
	callRoot := operand.callRoot
	producer, producerOK := schema.TailProducer(callRoot)
	applicationRoot, applicationRootOK := schema.CallRoot(operand.application)
	callID, callIDOK := key.ContentID()
	bodyID, bodyIDOK := operand.body.ContentID()
	outcomeRoot := operand.OutcomeRoot()
	outcomeID, outcomeIDOK := schema.RootID(outcomeRoot)
	planID, planIDOK := schema.RootID(planRoot)
	tailID, tailIDOK := schema.RootID(callRoot)
	if !keyOK || !key.IsApplication() || !applicationRootOK || !operand.outcome.Same(operand.outcome) || (operand.outcome.Kind() != flowkind.OutcomeNormal && operand.outcome.Kind() != flowkind.OutcomeReturn) || !operand.plan.Same(operand.plan) || operand.plan.Kind() != flowkind.OutcomeReturn || !planRootOK || !producerOK || producer.Kind() != packdomain.TailProducerCall || !schema.PackOnly(applicationRoot) || !schema.PackOnly(outcomeRoot) || !schema.PackOnly(planRoot) || !schema.PackOnly(callRoot) || !callIDOK || !bodyIDOK || !outcomeIDOK || !planIDOK || !tailIDOK {
		return false
	}
	expectedID := operationID("wippy.analysis.pack.body-return.v1\x00", []keyspace.ContentID{callID, bodyID, outcomeID, planID, tailID}, nil)
	return expectedID.Available() && expectedID == operand.id
}

func coldBodyReturnOperand(schema *packdomain.Schema, calls *calldomain.Algebra, operand BodyReturn) bool {
	var expected BodyReturn
	var ok bool
	if operand.outcome.Kind() == flowkind.OutcomeNormal {
		expected, ok = NewBodyNormalReturn(schema, calls, operand.application, operand.body)
	} else {
		expected, ok = NewBodyReturn(schema, calls, operand.application, operand.body)
	}
	return ok && expected == operand
}

func (rule *BodyReturnRule) operandContent(operand BodyReturn) (BodyReturn, [32]byte, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil || !coldBodyReturnOperand(rule.packs.Schema(), rule.calls.Algebra(), operand) {
		return BodyReturn{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.id), true
}

func (rule *BodyReturnRule) check(derivation engine.RuleDerivation[packdomain.Value, BodyReturn]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.packs == nil || rule.calls == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 2 || derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !coldBodyReturnOperand(rule.packs.Schema(), rule.calls.Algebra(), operand) || !derivation.OperandContentMatches([32]byte(operand.id)) {
		return engine.RuleEvidence{}, false
	}
	key, keyOK := rule.calls.Algebra().KeyForApplication(operand.application)
	callRef, callRefOK := rule.calls.Locate(key)
	outcomeRoot := operand.OutcomeRoot()
	packRef, packRefOK := rule.packs.Locate(outcomeRoot)
	outputRef, outputRefOK := rule.packs.Locate(operand.callRoot)
	disposition, dispositionOK := derivation.DispositionAt(0)
	callInput, callInputOK := derivation.InputAt(0)
	packInput, packInputOK := derivation.InputAt(1)
	if !keyOK || !callRefOK || !packRefOK || !outputRefOK || !dispositionOK || !callInputOK || !packInputOK || disposition.Guard().Empty() || !callInput.Guard().Same(disposition.Guard()) || !packInput.Guard().Same(disposition.Guard()) || !engine.DerivationReadMatchesRef(derivation, rule.callRead, callRef) || !engine.DerivationReadMatchesRef(derivation, rule.packRead, packRef) {
		return engine.RuleEvidence{}, false
	}
	callCells, callCellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.callRead)
	packCells, packCellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.packRead)
	if !callCellsOK || !packCellsOK || callCells.Count() != 1 || packCells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	callFact, callPresent, callAvailable := callCells.At(0)
	packFact, packPresent, packAvailable := packCells.At(0)
	if !callAvailable || !packAvailable {
		return engine.RuleEvidence{}, false
	}
	if !callPresent || !packPresent || callFact.IsEmpty() || packFact.IsBottom() || !callValueHasBody(callFact, operand.body) {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	expected, expectedOK := mapBodyReturn(rule.packs.Schema(), operand, packFact)
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if !expectedOK || expected.IsBottom() || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK || !engine.TargetMatchesRef(target, outputRef) || !rule.packs.Schema().Lattice().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
