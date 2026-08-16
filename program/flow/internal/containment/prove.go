package containment

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// emission is the private contract between owner-specific emitters and the
// containment kernel.  Emitters only produce candidate relations.  The
// kernel remains the sole authority that installs a parent, selects roots,
// rejects duplicate/conflicting rows, checks cycles, and builds intervals.
//
// static marks are a query projection, not another relation.  fallback rows
// are explicit deferred parent claims and are resolved by the same kernel;
// they are not a second graph or a retry path.
type emission struct {
	edges    []kernelEdge
	fallback []kernelEdge
	static   []keyspace.Term
	roots    []keyspace.Term
}

// Prove seals one canonical child-to-parent relation over the live authored
// Source, Flow, Static, and Module facets.  Every owner emits typed rows into
// one kernel input; no owner-specific graph or dense parent table survives.
func Prove(
	preimage source.Preimage,
	staticView static.View,
	view authored.View,
	bodies *body.Result,
	bindingResult binding.Result,
	moduleView module.View,
	entry keyspace.Term,
) (*Result, *StaticScopeProof, error) {
	// Fence the upstream structural owners before any cardinality walk or
	// emitter can dereference them. A nil Body and a foreign equal-cardinality
	// Body/Binding are equally unavailable to this proof.
	sourceID := preimage.Identity().ContentID()
	flowID := view.Cold().ContentID()
	if bodies == nil || !body.Matches(bodies, sourceID, flowID) || !binding.Matches(&bindingResult, sourceID, flowID) {
		return nil, nil, errors.New("program/flow/containment: Body or Binding provenance is unavailable or disagrees with Source or Flow")
	}
	counts, err := liveCounts(preimage, staticView, view, moduleView)
	if err != nil {
		return nil, nil, err
	}
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || !validTerm(entry, counts) {
		return nil, nil, errors.New("program/flow/containment: invalid Entry Body")
	}
	if err := validateOwnerCardinalities(preimage, staticView, view, bodies, bindingResult, moduleView, counts); err != nil {
		return nil, nil, err
	}

	// Every owner appends directly to this one construction buffer.  The
	// buffer is transferred to the kernel unchanged, so the coordinator never
	// keeps emitter-sized edge, fallback, or mark copies alongside it.
	emitted := emission{}
	if err := emitSource(preimage, staticView, view, bodies, counts, &emitted); err != nil {
		return nil, nil, err
	}
	if err := emitBodyBindingModule(view, bodies, bindingResult, moduleView, entry, counts, &emitted); err != nil {
		return nil, nil, err
	}

	// These are deliberately calls to the owner-specific verticals.  They do
	// not receive or return Result and cannot build a competing containment
	// image.  Their concrete implementations live with their semantic owner.
	flowExpressions, err := emitFlowExpressions(preimage, view, counts, &emitted)
	if err != nil {
		return nil, nil, err
	}
	emitted.edges = flowExpressions.edges
	if err := emitFlowExecution(view, counts, &emitted); err != nil {
		return nil, nil, err
	}
	resolver, err := emitStaticContainment(preimage, staticView, view, counts, &emitted)
	if err != nil {
		return nil, nil, err
	}

	result, err := buildKernel(kernelInput{
		counts:   counts,
		edges:    emitted.edges,
		fallback: emitted.fallback,
		roots:    emitted.roots,
		static:   emitted.static,
	})
	if err != nil {
		return nil, nil, err
	}
	staticID := staticView.ContentID()
	moduleID := moduleView.ContentID()
	result.sourceID = sourceID
	result.flowID = flowID
	result.staticID = staticID
	result.moduleID = moduleID
	if err := sealStaticStructuralRoles(result, staticView, moduleView); err != nil {
		return nil, nil, err
	}
	scopeProof, err := sealStaticScopeProof(preimage, staticView, view, staticID, moduleID, resolver, counts)
	if err != nil {
		return nil, nil, err
	}
	return result, scopeProof, nil
}

func liveCounts(preimage source.Preimage, staticView static.View, view authored.View, moduleView module.View) ([keyspace.FamilyCount]uint32, error) {
	var counts [keyspace.FamilyCount]uint32
	identity := preimage.Identity()
	if !identity.ContentID().Available() || identity.Name() == "" || identity.TermCount() == 0 ||
		!staticView.Available() || !view.Cold().ContentID().Available() || !moduleView.ContentID().Available() {
		return counts, errors.New("program/flow/containment: owner view expired")
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, errors.New("program/flow/containment: invalid Source family cardinality")
		}
		if family == keyspace.FamilyOutcome && count != 0 {
			return counts, errors.New("program/flow/containment: authored Outcome terms are forbidden")
		}
		counts[family] = uint32(count)
		total += uint64(count)
	}
	if total != uint64(identity.TermCount()) || total > uint64(^uint32(0)) {
		return counts, errors.New("program/flow/containment: Source family cardinality mismatch")
	}
	return counts, nil
}

func validateOwnerCardinalities(
	preimage source.Preimage,
	staticView static.View,
	view authored.View,
	bodies *body.Result,
	bindingResult binding.Result,
	moduleView module.View,
	counts [keyspace.FamilyCount]uint32,
) error {
	if err := validateSourceCounts(preimage, counts); err != nil {
		return err
	}
	if err := validateAuthoredCounts(view, bodies, counts); err != nil {
		return err
	}
	if err := validateStaticCardinalities(staticView, counts); err != nil {
		return err
	}
	if err := validateStaticCrossOwnerCardinalities(staticView, view, counts); err != nil {
		return err
	}
	if bindingResult.CellCount() != int(counts[keyspace.FamilyCell]) {
		return errors.New("program/flow/containment: Cell cardinality mismatch")
	}
	if moduleView.Count() != int(counts[keyspace.FamilyImport]) {
		return errors.New("program/flow/containment: Import cardinality mismatch")
	}
	return nil
}

func validateAuthoredCounts(view authored.View, bodies *body.Result, counts [keyspace.FamilyCount]uint32) error {
	checks := [...]struct {
		family keyspace.Family
		count  int
	}{
		{keyspace.FamilyValues, view.Values().Count()},
		{keyspace.FamilyLensExact, view.Access().Exact().Count()},
		{keyspace.FamilyLensKey, view.Access().Dynamic().Count()},
		{keyspace.FamilyReturn, view.Control().Returns().Count()},
		{keyspace.FamilyBreak, view.Control().Breaks().Count()},
		{keyspace.FamilyLabel, view.Control().Labels().Count()},
		{keyspace.FamilyGoto, view.Control().Gotos().Count()},
		{keyspace.FamilyBody, bodies.BodyCount()},
		{keyspace.FamilyCell, view.Storage().Cells().Count()},
		{keyspace.FamilyRead, view.Storage().Reads().Count()},
		{keyspace.FamilyVararg, view.Storage().Varargs().Count()},
		{keyspace.FamilyBind, view.Storage().Binds().Count()},
		{keyspace.FamilyAssign, view.Storage().Assigns().Count()},
		{keyspace.FamilyWrite, view.Storage().Writes().Count()},
		{keyspace.FamilyFunction, view.Functions().Count()},
		{keyspace.FamilyCall, view.Calls().Count()},
		{keyspace.FamilyBranch, view.Control().Branches().Count()},
		{keyspace.FamilyLoop, view.Control().Loops().Count()},
		{keyspace.FamilyTable, view.Tables().Count()},
		{keyspace.FamilyTableField, view.Fields().Count()},
		{keyspace.FamilyUnary, view.Operators().Unaries().Count()},
		{keyspace.FamilyBinary, view.Operators().Binaries().Count()},
		{keyspace.FamilySelect, view.Operators().Selects().Count()},
		{keyspace.FamilyValueClaim, view.Claims().Count()},
		{keyspace.FamilyTypeValue, view.TypeValues().Count()},
	}
	for _, check := range checks {
		if check.count < 0 || !keyspace.TermOrdinalFits(check.count) || uint32(check.count) != counts[check.family] {
			return errors.New("program/flow/containment: authored family cardinality mismatch")
		}
	}
	for _, family := range []keyspace.Family{
		keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyLabel,
		keyspace.FamilyGoto, keyspace.FamilyCell, keyspace.FamilyRead,
		keyspace.FamilyVararg, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyWrite, keyspace.FamilyFunction, keyspace.FamilyCall,
		keyspace.FamilyBranch, keyspace.FamilyLoop, keyspace.FamilyTable,
		keyspace.FamilyTableField, keyspace.FamilyUnary, keyspace.FamilyBinary,
		keyspace.FamilySelect, keyspace.FamilyValueClaim, keyspace.FamilyTypeValue,
	} {
		if err := validateAuthoredDenseFamily(view, family, counts[family]); err != nil {
			return err
		}
	}
	return nil
}

func validateAuthoredDenseFamily(view authored.View, family keyspace.Family, count uint32) error {
	for ordinal := uint32(1); ordinal <= count; ordinal++ {
		term := keyspace.MakeTerm(family, ordinal)
		var got keyspace.Term
		var ok bool
		index := int(ordinal - 1)
		switch family {
		case keyspace.FamilyValues:
			got, ok = view.Values().At(index)
		case keyspace.FamilyLensExact:
			got, ok = view.Access().Exact().At(index)
		case keyspace.FamilyLensKey:
			got, ok = view.Access().Dynamic().At(index)
		case keyspace.FamilyReturn:
			got, ok = view.Control().Returns().At(index)
		case keyspace.FamilyBreak:
			got, ok = view.Control().Breaks().At(index)
		case keyspace.FamilyLabel:
			got, ok = view.Control().Labels().At(index)
		case keyspace.FamilyGoto:
			got, ok = view.Control().Gotos().At(index)
		case keyspace.FamilyCell:
			got, ok = view.Storage().Cells().At(index)
		case keyspace.FamilyRead:
			got, ok = view.Storage().Reads().At(index)
		case keyspace.FamilyVararg:
			got, ok = view.Storage().Varargs().At(index)
		case keyspace.FamilyBind:
			got, ok = view.Storage().Binds().At(index)
		case keyspace.FamilyAssign:
			got, ok = view.Storage().Assigns().At(index)
		case keyspace.FamilyWrite:
			got, ok = view.Storage().Writes().At(index)
		case keyspace.FamilyFunction:
			got, ok = view.Functions().At(index)
		case keyspace.FamilyCall:
			got, ok = view.Calls().At(index)
		case keyspace.FamilyBranch:
			got, ok = view.Control().Branches().At(index)
		case keyspace.FamilyLoop:
			got, ok = view.Control().Loops().At(index)
		case keyspace.FamilyTable:
			got, ok = view.Tables().At(index)
		case keyspace.FamilyTableField:
			got, ok = view.Fields().At(index)
		case keyspace.FamilyUnary:
			got, ok = view.Operators().Unaries().At(index)
		case keyspace.FamilyBinary:
			got, ok = view.Operators().Binaries().At(index)
		case keyspace.FamilySelect:
			got, ok = view.Operators().Selects().At(index)
		case keyspace.FamilyValueClaim:
			got, ok = view.Claims().At(index)
		case keyspace.FamilyTypeValue:
			got, ok = view.TypeValues().At(index)
		default:
			return errors.New("program/flow/containment: unknown authored family")
		}
		if !ok || got != term {
			return errors.New("program/flow/containment: noncanonical authored ordinal")
		}
	}
	return nil
}

func validateStaticCardinalities(view static.View, counts [keyspace.FamilyCount]uint32) error {
	checks := [...]struct {
		family keyspace.Family
		count  int
	}{
		{keyspace.FamilyTypeAlias, view.Declarations().Aliases().Count()},
		{keyspace.FamilyTypeInterface, view.Declarations().Interfaces().Count()},
		{keyspace.FamilyTypeParam, view.Declarations().TypeParams().Count()},
		{keyspace.FamilyTypePrimitive, view.Types().Primitives().Count()},
		{keyspace.FamilyTypeLiteral, view.Types().Literals().Count()},
		{keyspace.FamilyTypeOptional, view.Types().Optionals().Count()},
		{keyspace.FamilyTypeUnion, view.Types().Unions().Count()},
		{keyspace.FamilyTypeIntersection, view.Types().Intersections().Count()},
		{keyspace.FamilyTypeRef, view.References().Count()},
		{keyspace.FamilyTypeGeneric, view.Types().Generics().Count()},
		{keyspace.FamilyTypeArray, view.Types().Arrays().Count()},
		{keyspace.FamilyTypeMap, view.Types().Maps().Count()},
		{keyspace.FamilyTypeRecord, view.Types().Records().Count()},
		{keyspace.FamilyTypeField, view.Types().Fields().Count()},
		{keyspace.FamilyTypeFunction, view.Signatures().TypeFunctions().Count()},
		{keyspace.FamilyTypeAsserts, view.Signatures().Assertions().Count()},
		{keyspace.FamilyDeclaredType, view.Declarations().DeclaredTypes().Count()},
		{keyspace.FamilyTypePublication, view.Publications().Count()},
		{keyspace.FamilyAnnotation, view.Operands().Annotations().Count()},
		{keyspace.FamilyTypeOf, view.Operators().TypeOfs().Count()},
		{keyspace.FamilyTypeKeyOf, view.Operators().KeyOfs().Count()},
		{keyspace.FamilyTypeIndexAccess, view.Operators().IndexAccesses().Count()},
		{keyspace.FamilyTypeConditional, view.Operators().Conditionals().Count()},
	}
	for _, check := range checks {
		if check.count < 0 || !keyspace.TermOrdinalFits(check.count) || uint32(check.count) != counts[check.family] {
			return errors.New("program/flow/containment: static family cardinality mismatch")
		}
	}
	return nil
}

func validTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 && ordinal <= counts[family]
}
