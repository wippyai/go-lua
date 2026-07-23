package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// RelationRuntimeOperand is one sealed runtime slot selected for a lowered
// occurrence. Value is an opaque capability token: only the production bridge
// that created the binding can interpret it as a formal operand slot.
type RelationRuntimeOperand struct {
	Role  AccessRole
	Value []byte
}

// BoundExternalCallFiber is one read-only formal-fiber inventory made
// available to the differential binder. It is not an execution dependency:
// normal relation evaluation has already frozen and published its own input
// row before this binder is ever constructed.
//
// Historical distinguishes a source point mentioned by the compiler-sealed
// ExternalCall access row from an ordinary provider wire. Ordinals are the
// body-local formal fibers retained by the frozen program for that point's
// publication coordinate.
type BoundExternalCallFiber struct {
	Point      cfg.Point
	Historical bool
	Ordinals   []uint32
}

// RelationBindingGap records a binder-local source that cannot be completed
// from the frozen program. It is deliberately observational: a gap neither
// widens the production dependency graph nor grants a synthetic operand.
type RelationBindingGap struct {
	Family     string
	Occurrence uint64
	Point      cfg.Point
	Reason     string
}

// BoundRelationOccurrence is the total real-occurrence binding record used by
// the Stage-3 bridge. It retains no solver callback or mutable tuple value.
type BoundRelationOccurrence struct {
	Ordinal            uint64
	Kind               OperatorKind
	Target             equation.Coordinate
	Operands           []RelationRuntimeOperand
	ExternalCallFibers []BoundExternalCallFiber
}

// RealRelationBodyBinding binds every frozen occurrence owned by one real
// production body. The entry State is accepted at this boundary solely to
// prove that the binding belongs to a concrete production invocation; State
// is never encoded into equation IR.
type RealRelationBodyBinding struct {
	program     *RelationProgram
	body        lexicalidentity.StableLexicalBodyID
	entry       state.State
	occurrences map[uint64]BoundRelationOccurrence
	contracts   map[equation.ContentID]OperatorContract
	gaps        []RelationBindingGap
}

// BindRealRelationBody constructs a total, read-only operand binder for body.
// It pre-enumerates the sealed template, so a subsequent lowering can neither
// invent an occurrence nor silently omit a runtime operand. Every slot token
// is body- and occurrence-qualified; the VM sees only its closed bytes.
func (p *RelationProgram) BindRealRelationBody(body lexicalidentity.StableLexicalBodyID, entry state.State) (*RealRelationBodyBinding, error) {
	if p == nil || p.formalTemplate == nil || !p.formalTemplate.validFor(p) {
		return nil, fmt.Errorf("transformer: real relation binder has no sealed relation template")
	}
	if _, ok := p.byBody[body]; !ok {
		return nil, fmt.Errorf("transformer: real relation binder has no body %s", body)
	}
	binding := &RealRelationBodyBinding{
		program: p, body: body, entry: entry,
		occurrences: make(map[uint64]BoundRelationOccurrence),
		contracts:   make(map[equation.ContentID]OperatorContract),
	}
	// CompileEquationIR is the sole occurrence walk.  This dry binder records
	// all real slots first; its drafts are intentionally discarded.
	compiler, err := Stage2EquationCompiler()
	if err != nil {
		return nil, err
	}
	_, err = p.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		if occurrence.Body != body {
			return relationPlaceholderDraft(p, occurrence)
		}
		bound, gaps, contract, bindErr := bindRealRelationOccurrence(p, occurrence)
		if bindErr != nil {
			return equation.Draft{}, bindErr
		}
		binding.occurrences[occurrence.Ordinal] = bound
		binding.contracts[equation.ContentID(contract.ContentID())] = contract
		binding.gaps = append(binding.gaps, gaps...)
		return relationDraftFromBoundOccurrence(occurrence, bound, contract)
	})
	if err != nil {
		return nil, err
	}
	if len(binding.occurrences) == 0 {
		return nil, fmt.Errorf("transformer: real relation binder found no occurrences for %s", body)
	}
	return binding, nil
}

// BindingGaps returns the named fail-closed omissions discovered while the
// differential binder completed its observation-only fiber inventory.
func (b *RealRelationBodyBinding) BindingGaps() []RelationBindingGap {
	if b == nil {
		return nil
	}
	out := append([]RelationBindingGap(nil), b.gaps...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Occurrence != out[j].Occurrence {
			return out[i].Occurrence < out[j].Occurrence
		}
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		if out[i].Point != out[j].Point {
			return out[i].Point < out[j].Point
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

// Occurrences returns detached copies in canonical occurrence order.
func (b *RealRelationBodyBinding) Occurrences() []BoundRelationOccurrence {
	if b == nil {
		return nil
	}
	ordinals := make([]uint64, 0, len(b.occurrences))
	for ordinal := range b.occurrences {
		ordinals = append(ordinals, ordinal)
	}
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	out := make([]BoundRelationOccurrence, 0, len(ordinals))
	for _, ordinal := range ordinals {
		occurrence := b.occurrences[ordinal]
		occurrence.Operands = cloneRelationRuntimeOperands(occurrence.Operands)
		occurrence.ExternalCallFibers = cloneBoundExternalCallFibers(occurrence.ExternalCallFibers)
		out = append(out, occurrence)
	}
	return out
}

// Compile lowers this body only through the complete Stage-2 catalog using
// this binding's exact occurrence contracts and slots.
func (b *RealRelationBodyBinding) Compile() (equation.Artifact, error) {
	if b == nil || b.program == nil {
		return equation.Artifact{}, fmt.Errorf("transformer: real relation binder is unowned")
	}
	compiler, err := Stage2EquationCompiler()
	if err != nil {
		return equation.Artifact{}, err
	}
	return b.program.CompileBodyEquationIR(b.body, compiler, b.Binder())
}

// Binder returns the exact total binder for this concrete body. Any occurrence
// absent from the pre-enumerated production binding names itself in the error.
func (b *RealRelationBodyBinding) Binder() RelationEquationBinder {
	return func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		if b == nil || b.program == nil || occurrence.Body != b.body {
			return equation.Draft{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
		}
		bound, ok := b.occurrences[occurrence.Ordinal]
		if !ok || bound.Kind != occurrence.Kind {
			return equation.Draft{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
		}
		contract, ok := b.contractsForOccurrence(occurrence.Ordinal)
		if !ok {
			return equation.Draft{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
		}
		return relationDraftFromBoundOccurrence(occurrence, bound, contract)
	}
}

func (b *RealRelationBodyBinding) contractsForOccurrence(ordinal uint64) (OperatorContract, bool) {
	occurrence, ok := b.occurrences[ordinal]
	if !ok {
		return OperatorContract{}, false
	}
	for id, contract := range b.contracts {
		if id == equation.ContentID(contract.ContentID()) && contract.Kind == occurrence.Kind && contract.Occurrence.Ordinal() == ordinal {
			return contract, true
		}
	}
	return OperatorContract{}, false
}

func bindRealRelationOccurrence(program *RelationProgram, occurrence RelationEquationOccurrence) (BoundRelationOccurrence, []RelationBindingGap, OperatorContract, error) {
	if occurrence.Ordinal == 0 || occurrence.Body == (lexicalidentity.StableLexicalBodyID{}) || occurrence.Kind == "" {
		return BoundRelationOccurrence{}, nil, OperatorContract{}, fmt.Errorf("transformer: unbound occurrence %s/%d", occurrence.Body, occurrence.Ordinal)
	}
	contract, err := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, occurrence.Ordinal))
	if err != nil {
		return BoundRelationOccurrence{}, nil, OperatorContract{}, err
	}
	body := equation.BodyID(occurrence.Body)
	bound := BoundRelationOccurrence{
		Ordinal:  occurrence.Ordinal,
		Kind:     occurrence.Kind,
		Target:   equation.Coordinate{Body: body, Name: fmt.Sprintf("occurrence-%d", occurrence.Ordinal)},
		Operands: make([]RelationRuntimeOperand, 0, len(contract.Operands)),
	}
	for _, role := range contract.Operands {
		bound.Operands = append(bound.Operands, RelationRuntimeOperand{
			Role: role, Value: []byte(fmt.Sprintf("relation-runtime-slot/v1/%x/%d/%s", occurrence.Body, occurrence.Ordinal, role)),
		})
	}
	fibers, gaps, err := completeExternalCallBinderFibers(program, occurrence)
	if err != nil {
		return BoundRelationOccurrence{}, nil, OperatorContract{}, err
	}
	bound.ExternalCallFibers = fibers
	return bound, gaps, contract, nil
}

// completeExternalCallBinderFibers is the scoped completion seam for Item 2.
// It computes a differential-only view from frozen syntax and the already
// frozen tuple inventory. In particular, it must never call region inventory
// construction, alter publication, or add an ExternalCall input dependency.
func completeExternalCallBinderFibers(program *RelationProgram, occurrence RelationEquationOccurrence) ([]BoundExternalCallFiber, []RelationBindingGap, error) {
	if occurrence.Kind != OperatorExternalCall {
		return nil, nil, nil
	}
	if program == nil || program.formalFibers == nil || occurrence.operator.externalCall == nil ||
		occurrence.cell.Variable == 0 || int(occurrence.cell.Variable) > len(program.bodies) {
		return nil, nil, fmt.Errorf("transformer: external-call binder has no frozen fiber authority")
	}
	plan := occurrence.operator.externalCall
	body := &program.bodies[occurrence.cell.Variable-1]
	span, ok := program.formalFibers.span(occurrence.cell.Variable)
	if !ok || body.relation.code == nil {
		return nil, nil, fmt.Errorf("transformer: external-call binder has no frozen body span")
	}

	byPoint := make(map[cfg.Point]BoundExternalCallFiber)
	add := func(point cfg.Point, historical bool, ordinals []formalFiberOrdinal) {
		fiber := byPoint[point]
		fiber.Point = point
		fiber.Historical = fiber.Historical || historical
		for _, ordinal := range ordinals {
			if ordinal >= 0 {
				fiber.Ordinals = append(fiber.Ordinals, uint32(ordinal))
			}
		}
		fiber.Ordinals = canonicalBoundExternalCallOrdinals(fiber.Ordinals)
		byPoint[point] = fiber
	}
	var visitProvider func(formalExternalCallProvider)
	visitProvider = func(provider formalExternalCallProvider) {
		for _, wire := range provider.wires {
			add(wire.point, false, wire.ordinals)
		}
		for _, child := range provider.children {
			visitProvider(child)
		}
	}
	visitProvider(plan.provider)

	// A point-qualified access is a historical input only when it is not the
	// call point itself. The complete body span is its binder-local inventory:
	// the production equation owns the point coordinate, while the differential
	// observer merely needs the sealed fibers it may bind to that coordinate.
	allOrdinals := make([]formalFiberOrdinal, 0, len(span.descriptors()))
	for _, descriptor := range span.descriptors() {
		ordinal, exact := span.ordinal(descriptor)
		if !exact {
			return nil, nil, fmt.Errorf("transformer: external-call binder has an unaddressable frozen fiber")
		}
		allOrdinals = append(allOrdinals, ordinal)
	}
	var gaps []RelationBindingGap
	seenHistorical := make(map[cfg.Point]struct{})
	for _, access := range plan.access {
		if !access.hasPoint || access.point == plan.point {
			continue
		}
		if _, seen := seenHistorical[access.point]; seen {
			continue
		}
		seenHistorical[access.point] = struct{}{}
		if !externalCallBinderHasPublication(body.relation.code, occurrence.cell.Variable, access.point) {
			gaps = append(gaps, RelationBindingGap{
				Family: "external-call-historical-fiber", Occurrence: occurrence.Ordinal, Point: access.point,
				Reason: "frozen program has no published output for historical point",
			})
			continue
		}
		add(access.point, true, allOrdinals)
	}

	points := make([]cfg.Point, 0, len(byPoint))
	for point := range byPoint {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	out := make([]BoundExternalCallFiber, 0, len(points))
	for _, point := range points {
		out = append(out, byPoint[point])
	}
	return out, gaps, nil
}

func externalCallBinderHasPublication(code *relationCode, variable relationVar, point cfg.Point) bool {
	if code == nil || variable == 0 {
		return false
	}
	for _, publication := range code.publication.points {
		if publication.point != point {
			continue
		}
		_, dependency, valid := formalRelationPublishedOutputCell(variable, code, publication.ref)
		if valid && dependency {
			return true
		}
	}
	return false
}

func canonicalBoundExternalCallOrdinals(in []uint32) []uint32 {
	out := append([]uint32(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	write := 0
	for _, ordinal := range out {
		if write == 0 || out[write-1] != ordinal {
			out[write] = ordinal
			write++
		}
	}
	return out[:write]
}

func relationDraftFromBoundOccurrence(occurrence RelationEquationOccurrence, bound BoundRelationOccurrence, contract OperatorContract) (equation.Draft, error) {
	body := equation.BodyID(occurrence.Body)
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	operands := make([]equation.Operand, 0, len(bound.Operands))
	for _, operand := range bound.Operands {
		term := equation.ClosedTerm(operand.Value)
		if operand.Role == AccessEntry {
			term = equation.EntryTerm(entry)
		}
		operands = append(operands, equation.Operand{Role: string(operand.Role), Term: term})
	}
	return equation.Draft{
		Target: bound.Target, Entry: entry,
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, nil
}

// relationPlaceholderDraft keeps the dry whole-template walk fail-closed for
// foreign bodies without granting their binding to this body.
func relationPlaceholderDraft(program *RelationProgram, occurrence RelationEquationOccurrence) (equation.Draft, error) {
	bound, _, contract, err := bindRealRelationOccurrence(program, occurrence)
	if err != nil {
		return equation.Draft{}, err
	}
	return relationDraftFromBoundOccurrence(occurrence, bound, contract)
}

func cloneRelationRuntimeOperands(in []RelationRuntimeOperand) []RelationRuntimeOperand {
	out := make([]RelationRuntimeOperand, len(in))
	for index, operand := range in {
		out[index] = RelationRuntimeOperand{Role: operand.Role, Value: append([]byte(nil), operand.Value...)}
	}
	return out
}

func cloneBoundExternalCallFibers(in []BoundExternalCallFiber) []BoundExternalCallFiber {
	out := make([]BoundExternalCallFiber, len(in))
	for index, fiber := range in {
		out[index] = BoundExternalCallFiber{
			Point:      fiber.Point,
			Historical: fiber.Historical,
			Ordinals:   append([]uint32(nil), fiber.Ordinals...),
		}
	}
	return out
}
