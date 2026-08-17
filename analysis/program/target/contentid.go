package target

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Version 22 carries the complete neutral type-contract declaration for each
// frozen type row, including primitive identity and external formal scope.
// Version 21 adds explicit publication-effect presence and typed descriptor
// bytes to each effect row. A target identity from any preceding layout must
// never be reused for a contract with publication semantics.
// Version 20 adds the exact initial whole-object immutable header to every
// boot shape. A target identity from any preceding layout must never be reused
// for a different bootstrap Heap header.
// Version 19 adds the closed string.gsub table-replacement branch.
// Version 18 adds retained callback-holder protocol rows and the mandatory
// zero-holder branch of a callback release. A target identity from any
// preceding layout must never be reused as this schema.
const contentIDCodecVersion = 22

// ContentID derives the SHA-256 identity of the complete observable sealed
// contract. It encodes no authoring references, Go object identities, lookup
// indices, capacities, or other derived implementation caches.
func (c *Contract) ContentID() (id identity.ContentID) {
	// Contract is intentionally unconstructable outside this package, but this
	// boundary must still fail closed for a partially assembled internal value.
	// The recovery also keeps a future decoder bug from publishing a digest for
	// a panicking malformed table.
	defer func() {
		if recover() != nil {
			id = identity.ContentID{}
		}
	}()
	if c == nil || !c.sealed || c.opaque == 0 || uint64(c.opaque) != uint64(len(c.operations)) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	if err := encodeContractCanonical(hash, c); err != nil {
		return identity.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// Record kinds are part of target-contract/v1. They are intentionally local
// to this semantic codec; framing.Writer does not know target's schema.
const (
	recordContract uint64 = iota + 1
	recordOperation
	recordBinding
	recordValues
	recordOutcome
	recordCallback
	recordSubedge
	recordCallbackRelease
	recordSuspension
	recordSpawn
	recordResume
	recordTransfer
	recordEffect
	recordProduced
	recordCapture
	recordCallbackResult
	recordResultAlias
	recordProtocol
	recordState
	recordAcquisition
	recordTransition
	recordTransitionOutcome
	recordEscape
	recordProtocolCallbackHolder
	recordInitialRoot
	recordBootShape
	recordInitialEntry
	recordInitialBinding
	recordInitialValue
	recordFreshResult
	recordInitialMetatableAttachment
	recordGsubTableReplacement
)

func encodeContractCanonical(dst interface{ Write([]byte) (int, error) }, c *Contract) error {
	var w framing.Writer
	if err := w.Reset(dst, "program/target-contract", contentIDCodecVersion); err != nil {
		return err
	}
	if err := encodeContract(&w, c); err != nil {
		return err
	}
	return w.Finish()
}

func encodeContract(w *framing.Writer, c *Contract) error {
	if c == nil {
		return errors.New("target: unavailable contract")
	}
	if err := w.Record(recordContract); err != nil {
		return err
	}
	operations := c.OperationCount()
	if err := w.Count(uint64(operations)); err != nil {
		return err
	}
	for index := 0; index < operations; index++ {
		op, ok := c.OperationAt(index)
		if !ok {
			return errors.New("target: malformed operation table")
		}
		if err := encodeOperation(w, c, op); err != nil {
			return err
		}
	}
	protocols := c.ProtocolCount()
	if err := w.Count(uint64(protocols)); err != nil {
		return err
	}
	for index := 0; index < protocols; index++ {
		protocol, ok := c.ProtocolAt(index)
		if !ok {
			return errors.New("target: malformed protocol table")
		}
		if err := encodeProtocol(w, c, protocol); err != nil {
			return err
		}
	}
	if err := encodeBoot(w, c); err != nil {
		return err
	}
	return nil
}

func encodeBoot(w *framing.Writer, c *Contract) error {
	roots := c.InitialRootCount()
	if err := w.Count(uint64(roots)); err != nil {
		return err
	}
	for index := 0; index < roots; index++ {
		root, ok := c.InitialRootAt(index)
		if !ok {
			return errors.New("target: malformed initial root")
		}
		identity, ok := c.InitialRootIdentity(root)
		if !ok {
			return errors.New("target: malformed initial root identity")
		}
		shape, ok := c.InitialRootBootShape(root)
		if !ok {
			return errors.New("target: malformed initial root shape")
		}
		shapeRoot, ok := c.BootShapeRoot(shape)
		if !ok || shapeRoot != root {
			return errors.New("target: malformed boot shape root")
		}
		aggregate, ok := c.BootShapeAggregate(shape)
		if !ok {
			return errors.New("target: malformed boot shape aggregate")
		}
		immutable, ok := c.BootShapeImmutable(shape)
		if !ok {
			return errors.New("target: malformed boot shape immutable header")
		}
		value, ok := c.BootShapeValue(shape)
		if !ok {
			return errors.New("target: malformed boot shape value")
		}
		if err := w.Record(recordInitialRoot); err != nil {
			return err
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := w.String(identity); err != nil {
			return err
		}
		if err := w.Record(recordBootShape); err != nil {
			return err
		}
		if err := w.Uint(uint64(shape)); err != nil {
			return err
		}
		if err := w.Uint(uint64(aggregate)); err != nil {
			return err
		}
		if err := w.Bool(immutable); err != nil {
			return err
		}
		if err := encodeInitialValue(w, c, value); err != nil {
			return err
		}
	}
	entries := c.InitialEntryCount()
	if err := w.Count(uint64(entries)); err != nil {
		return err
	}
	for index := 0; index < entries; index++ {
		root, key, value, mutability, ok := c.InitialEntryAt(index)
		if !ok {
			return errors.New("target: malformed initial entry")
		}
		if err := w.Record(recordInitialEntry); err != nil {
			return err
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
		if err := w.Uint(uint64(mutability)); err != nil {
			return err
		}
		if err := encodeInitialValue(w, c, value); err != nil {
			return err
		}
	}
	bindings := c.InitialBindingCount()
	if err := w.Count(uint64(bindings)); err != nil {
		return err
	}
	for index := 0; index < bindings; index++ {
		name, class, value, root, key, ok := c.InitialBindingAt(index)
		if !ok {
			return errors.New("target: malformed initial binding")
		}
		if err := w.Record(recordInitialBinding); err != nil {
			return err
		}
		if err := w.String(name); err != nil {
			return err
		}
		if err := w.Uint(uint64(class)); err != nil {
			return err
		}
		if err := encodeInitialValue(w, c, value); err != nil {
			return err
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	}
	attachments := c.InitialMetatableAttachmentCount()
	if err := w.Count(uint64(attachments)); err != nil {
		return err
	}
	for index := 0; index < attachments; index++ {
		base, metatable, ok := c.InitialMetatableAttachmentAt(index)
		if !ok {
			return errors.New("target: malformed initial metatable attachment")
		}
		if err := w.Record(recordInitialMetatableAttachment); err != nil {
			return err
		}
		if err := w.Uint(uint64(base)); err != nil {
			return err
		}
		if err := w.Uint(uint64(metatable)); err != nil {
			return err
		}
	}
	return nil
}

func encodeExactKey(w *framing.Writer, c *Contract, key ExactKey) error {
	value, ok := c.ExactKeyValue(key)
	if !ok {
		return errors.New("target: malformed exact key")
	}
	if err := w.Uint(uint64(value.Kind)); err != nil {
		return err
	}
	switch value.Kind {
	case keyspace.LiteralBool:
		return w.Bool(value.Bool)
	case keyspace.LiteralInteger:
		// The canonical writer carries unsigned scalars only. Reinterpreting the
		// two's-complement bit pattern is a total, injective encoding of int64;
		// the preceding literal kind keeps it separate from all other payloads.
		return w.Uint(uint64(value.Integer))
	case keyspace.LiteralFloat:
		return w.Uint(value.FloatBits)
	case keyspace.LiteralString:
		return w.String(value.String)
	default:
		return errors.New("target: malformed exact key kind")
	}
}

func encodeInitialValue(w *framing.Writer, c *Contract, value InitialValue) error {
	kind, ok := c.InitialValueKind(value)
	if !ok {
		return errors.New("target: malformed initial value")
	}
	if err := w.Record(recordInitialValue); err != nil {
		return err
	}
	if err := w.Uint(uint64(kind)); err != nil {
		return err
	}
	switch kind {
	case InitialValueNil, InitialValueAbsent:
		return nil
	case InitialValueBoolean:
		item, ok := c.InitialValueBoolean(value)
		if !ok {
			return errors.New("target: malformed initial boolean")
		}
		return w.Bool(item)
	case InitialValueInteger:
		item, ok := c.InitialValueInteger(value)
		if !ok {
			return errors.New("target: malformed initial integer")
		}
		return w.Uint(uint64(item))
	case InitialValueFloat:
		item, ok := c.InitialValueFloatBits(value)
		if !ok {
			return errors.New("target: malformed initial float")
		}
		return w.Uint(item)
	case InitialValueString:
		item, ok := c.InitialValueString(value)
		if !ok {
			return errors.New("target: malformed initial string")
		}
		return w.String(item)
	case InitialValueRoot:
		item, ok := c.InitialValueRoot(value)
		if !ok {
			return errors.New("target: malformed initial root value")
		}
		return w.Uint(uint64(item))
	case InitialValueOperation:
		item, ok := c.InitialValueOperation(value)
		if !ok {
			return errors.New("target: malformed initial operation value")
		}
		return w.Uint(uint64(item))
	case InitialValueDeniedOperation:
		namespace, ok := c.InitialValueDeniedNamespace(value)
		if !ok {
			return errors.New("target: malformed denied initial operation")
		}
		if err := w.Uint(uint64(namespace)); err != nil {
			return err
		}
		owner := c.InitialValueDeniedOwnerCount(value)
		if err := w.Count(uint64(owner)); err != nil {
			return err
		}
		for index := 0; index < owner; index++ {
			part, ok := c.InitialValueDeniedOwnerKeyAt(value, index)
			if !ok {
				return errors.New("target: malformed denied initial owner")
			}
			if err := encodeExactKey(w, c, part); err != nil {
				return err
			}
		}
		member := c.InitialValueDeniedMemberCount(value)
		if err := w.Count(uint64(member)); err != nil {
			return err
		}
		for index := 0; index < member; index++ {
			part, ok := c.InitialValueDeniedMemberKeyAt(value, index)
			if !ok {
				return errors.New("target: malformed denied initial member")
			}
			if err := encodeExactKey(w, c, part); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("target: invalid initial value kind")
	}
}

func encodeOperation(w *framing.Writer, c *Contract, op Operation) error {
	if err := w.Record(recordOperation); err != nil {
		return err
	}
	if err := w.Uint(uint64(op)); err != nil {
		return err
	}

	bindings := c.BindingCount(op)
	if err := w.Count(uint64(bindings)); err != nil {
		return err
	}
	for index := 0; index < bindings; index++ {
		if err := w.Record(recordBinding); err != nil {
			return err
		}
		namespace, ok := c.BindingNamespaceAt(op, index)
		if !ok {
			return errors.New("target: malformed binding")
		}
		if err := w.Uint(uint64(namespace)); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, index, true); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, index, false); err != nil {
			return err
		}
	}

	formals := c.TypeFormalCount(op)
	if err := w.Count(uint64(formals)); err != nil {
		return err
	}
	for index := 0; index < formals; index++ {
		constraint, found := c.TypeFormalConstraint(op, TypeFormal(index))
		if err := w.Bool(found); err != nil {
			return err
		}
		if found {
			if err := encodeType(w, c, constraint); err != nil {
				return err
			}
		}
	}
	if err := w.Uint(uint64(c.ValuesVarCount(op))); err != nil {
		return err
	}
	for variable := 0; variable < c.ValuesVarCount(op); variable++ {
		class, found := c.ValuesVarType(op, ValuesVar(variable))
		if !found {
			return errors.New("target: malformed Values variable type")
		}
		if err := encodeType(w, c, class); err != nil {
			return err
		}
	}
	if err := w.Uint(uint64(c.RowFormalCount(op))); err != nil {
		return err
	}
	input, ok := c.Input(op)
	if !ok {
		return errors.New("target: malformed input Values")
	}
	if err := encodeValues(w, c, input); err != nil {
		return err
	}

	callbacks := c.CallbackCount(op)
	if err := w.Count(uint64(callbacks)); err != nil {
		return err
	}
	for index := 0; index < callbacks; index++ {
		id, found := c.CallbackAt(op, index)
		if !found {
			return errors.New("target: malformed callback")
		}
		owner, found := c.CallbackOwner(id)
		if !found || owner != op {
			return errors.New("target: malformed callback owner")
		}
		if err := w.Record(recordCallback); err != nil {
			return err
		}
		if err := w.Uint(uint64(id)); err != nil {
			return err
		}
		source, found := c.CallbackFunction(id)
		if !found {
			return errors.New("target: malformed callback source")
		}
		if err := encodeCoordinate(w, uint64(source.Kind), uint64(source.Ordinal)); err != nil {
			return err
		}
		arguments, found := c.CallbackArguments(id)
		if !found {
			return errors.New("target: malformed callback arguments")
		}
		if err := encodeValues(w, c, arguments); err != nil {
			return err
		}
		admission, found := c.CallbackAdmission(id)
		if !found || !validAdmission(admission) {
			return errors.New("target: malformed callback admission")
		}
		if err := w.Uint(uint64(admission)); err != nil {
			return err
		}
		for _, kind := range [...]flowkind.OutcomeKind{
			flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
			flowkind.OutcomeYield, flowkind.OutcomeCancel,
		} {
			values, found := c.CallbackOutcome(id, kind)
			if !found {
				return errors.New("target: malformed callback outcome")
			}
			if err := w.Uint(uint64(kind)); err != nil {
				return err
			}
			if err := encodeValues(w, c, values); err != nil {
				return err
			}
		}
		lifecycle, found := c.CallbackLifecycle(id)
		if !found {
			return errors.New("target: malformed callback lifecycle")
		}
		if err := w.Uint(uint64(lifecycle)); err != nil {
			return err
		}
		tail, variable, found := c.CallbackEffectTail(id)
		if !found {
			return errors.New("target: malformed callback effect tail")
		}
		if err := w.Uint(uint64(tail)); err != nil {
			return err
		}
		if err := w.Uint(uint64(variable)); err != nil {
			return err
		}
		effects := c.CallbackEffectCount(id)
		if err := w.Count(uint64(effects)); err != nil {
			return err
		}
		for effect := 0; effect < effects; effect++ {
			row, ok := c.callbackEffect(id, effect)
			if !ok {
				return errors.New("target: malformed callback effect")
			}
			if err := encodeEffectRow(w, c, row); err != nil {
				return err
			}
		}
		releaseOperation, releaseInput, releaseOutcome, releaseMode, hasRelease := c.CallbackRelease(id)
		if err := w.Bool(hasRelease); err != nil {
			return err
		}
		if hasRelease {
			if err := w.Record(recordCallbackRelease); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseOperation)); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseInput)); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseOutcome)); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseMode)); err != nil {
				return err
			}
			zeroBehavior, zeroOutcome, zeroOK := c.CallbackReleaseZero(id)
			if !zeroOK || !validCallbackReleaseZeroBehavior(zeroBehavior) {
				return errors.New("target: malformed callback release zero behavior")
			}
			if err := w.Uint(uint64(zeroBehavior)); err != nil {
				return err
			}
			switch zeroBehavior {
			case CallbackReleaseZeroThrow, CallbackReleaseZeroIdempotent:
				if err := w.Uint(uint64(zeroOutcome)); err != nil {
					return err
				}
			case CallbackReleaseZeroSuppress:
				if zeroOutcome != 0 {
					return errors.New("target: suppressed callback release retained an outcome")
				}
			default:
				return errors.New("target: malformed callback release zero behavior")
			}
		}
	}

	subedges := c.SubedgeCount(op)
	if err := w.Count(uint64(subedges)); err != nil {
		return err
	}
	for index := 0; index < subedges; index++ {
		edge, found := c.SubedgeAt(op, index)
		if !found {
			return errors.New("target: malformed subedge")
		}
		if err := encodeSubedge(w, c, op, edge); err != nil {
			return err
		}
	}

	outcomes := c.OutcomeCount(op)
	if err := w.Count(uint64(outcomes)); err != nil {
		return err
	}
	for index := 0; index < outcomes; index++ {
		if err := encodeOutcome(w, c, op, index); err != nil {
			return err
		}
	}

	suspensions := c.SuspensionCount(op)
	if err := w.Count(uint64(suspensions)); err != nil {
		return err
	}
	for index := 0; index < suspensions; index++ {
		yield, reentry, source, multiplicity, found := c.SuspensionAt(op, index)
		if !found {
			return errors.New("target: malformed suspension")
		}
		if err := w.Record(recordSuspension); err != nil {
			return err
		}
		if err := w.Uint(uint64(yield)); err != nil {
			return err
		}
		if err := w.Uint(uint64(reentry)); err != nil {
			return err
		}
		if err := w.Uint(uint64(source)); err != nil {
			return err
		}
		if err := w.Uint(uint64(multiplicity)); err != nil {
			return err
		}
	}
	spawns := c.SpawnCount(op)
	if err := w.Count(uint64(spawns)); err != nil {
		return err
	}
	for index := 0; index < spawns; index++ {
		spawn, found := c.SpawnIDAt(op, index)
		if !found {
			return errors.New("target: malformed spawn")
		}
		owner, function, child, yield, resume, entry, resumeValues, found := c.Spawn(spawn)
		if !found || owner != op {
			return errors.New("target: malformed spawn")
		}
		if err := w.Record(recordSpawn); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(function.Kind), uint64(function.Ordinal)); err != nil {
			return err
		}
		for _, value := range []uint64{uint64(child), uint64(yield), uint64(resume), uint64(entry), uint64(resumeValues)} {
			if err := w.Uint(value); err != nil {
				return err
			}
		}
		alternatives := c.SpawnSiblingCount(spawn)
		if err := w.Count(uint64(alternatives)); err != nil {
			return err
		}
		for sibling := 0; sibling < alternatives; sibling++ {
			alternative, found := c.SpawnSiblingAt(spawn, sibling)
			if !found {
				return errors.New("target: malformed spawn sibling")
			}
			if err := w.Uint(uint64(alternative)); err != nil {
				return err
			}
		}
	}
	resumes := c.ResumeCount(op)
	if err := w.Count(uint64(resumes)); err != nil {
		return err
	}
	for index := 0; index < resumes; index++ {
		resume, found := c.ResumeIDAt(op, index)
		if !found {
			return errors.New("target: malformed resume")
		}
		owner, source, carrier, arguments, found := c.Resume(resume)
		if !found || owner != op {
			return errors.New("target: malformed resume")
		}
		if err := w.Record(recordResume); err != nil {
			return err
		}
		if err := w.Uint(uint64(source)); err != nil {
			return err
		}
		if err := w.Uint(uint64(carrier)); err != nil {
			return err
		}
		if err := encodeValues(w, c, arguments); err != nil {
			return err
		}
		outcomes := c.ResumeOutcomeCount(resume)
		if err := w.Count(uint64(outcomes)); err != nil {
			return err
		}
		for outcome := 0; outcome < outcomes; outcome++ {
			kind, targetOutcome, found := c.ResumeOutcomeAt(resume, outcome)
			if !found {
				return errors.New("target: malformed resume outcome")
			}
			if err := w.Uint(uint64(kind)); err != nil {
				return err
			}
			if err := w.Uint(uint64(targetOutcome)); err != nil {
				return err
			}
		}
	}

	transfers := c.TransferCount(op)
	if err := w.Count(uint64(transfers)); err != nil {
		return err
	}
	for index := 0; index < transfers; index++ {
		endpoint, found := c.TransferEndpointAt(op, index)
		if !found {
			return errors.New("target: malformed transfer")
		}
		if err := w.Record(recordTransfer); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(endpoint.Kind), uint64(endpoint.Input)); err != nil {
			return err
		}
		payload, found := c.TransferPayloadAt(op, index)
		if !found {
			return errors.New("target: malformed transfer payload")
		}
		if err := encodeCoordinate(w, uint64(payload.Kind), uint64(payload.Ordinal)); err != nil {
			return err
		}
		alias, found := c.TransferAliasAt(op, index)
		if !found {
			return errors.New("target: malformed transfer alias")
		}
		if err := encodeCoordinate(w, uint64(alias.Kind), uint64(alias.Ordinal)); err != nil {
			return err
		}
		identity, found := c.TransferIdentityAt(op, index)
		if !found {
			return errors.New("target: malformed transfer identity")
		}
		if err := w.Uint(uint64(identity)); err != nil {
			return err
		}
		capabilities, found := c.TransferCapabilitiesAt(op, index)
		if !found {
			return errors.New("target: malformed transfer capabilities")
		}
		if err := w.Uint(uint64(capabilities)); err != nil {
			return err
		}
		count := c.TransferOutcomeCount(op, index)
		if err := w.Count(uint64(count)); err != nil {
			return err
		}
		for item := 0; item < count; item++ {
			outcome, possibility, found := c.TransferOutcomeAt(op, index, item)
			if !found {
				return errors.New("target: malformed transfer outcome")
			}
			if err := w.Uint(uint64(outcome)); err != nil {
				return err
			}
			if err := w.Uint(uint64(possibility)); err != nil {
				return err
			}
		}
	}

	tail, variable, found := c.EffectTail(op)
	if !found {
		return errors.New("target: malformed effect tail")
	}
	if err := w.Uint(uint64(tail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(variable)); err != nil {
		return err
	}
	effects := c.EffectCount(op)
	if err := w.Count(uint64(effects)); err != nil {
		return err
	}
	for index := 0; index < effects; index++ {
		if err := encodeEffect(w, c, op, index); err != nil {
			return err
		}
	}
	if replacement, key, access, resultOutcome, result, present := c.GsubTableReplacement(op); present {
		if err := w.Bool(true); err != nil {
			return err
		}
		if err := w.Record(recordGsubTableReplacement); err != nil {
			return err
		}
		for _, value := range []uint64{uint64(replacement), uint64(key), uint64(resultOutcome), uint64(result)} {
			if err := w.Uint(value); err != nil {
				return err
			}
		}
		role, found := c.SubedgeRole(access)
		if !found || role == 0 {
			return errors.New("target: malformed gsub table access")
		}
		if err := w.Uint(uint64(role)); err != nil {
			return err
		}
		aliases := c.GsubTableReplacementEffectAliasCount(op)
		if err := w.Count(uint64(aliases)); err != nil {
			return err
		}
		for index := 0; index < aliases; index++ {
			effect, found := c.GsubTableReplacementEffectAliasAt(op, index)
			if !found {
				return errors.New("target: malformed gsub table effect alias")
			}
			if err := w.Uint(uint64(effect)); err != nil {
				return err
			}
		}
	} else if err := w.Bool(false); err != nil {
		return err
	}
	return nil
}

func encodeBindingSegments(w *framing.Writer, c *Contract, op Operation, binding int, owner bool) error {
	count := c.BindingMemberCountAt(op, binding)
	if owner {
		count = c.BindingOwnerCountAt(op, binding)
	}
	if err := w.Count(uint64(count)); err != nil {
		return err
	}
	for index := 0; index < count; index++ {
		var value ExactKey
		var ok bool
		if owner {
			value, ok = c.BindingOwnerKeyAt(op, binding, index)
		} else {
			value, ok = c.BindingMemberKeyAt(op, binding, index)
		}
		if !ok {
			return errors.New("target: malformed binding segment")
		}
		if err := encodeExactKey(w, c, value); err != nil {
			return err
		}
	}
	return nil
}

// encodeSubedge writes the sole internal-application relation in terms of
// semantic roles and full Values endpoints. It deliberately never encodes a
// Values handle as if handle equality were a flow edge.
func encodeSubedge(w *framing.Writer, c *Contract, owner Operation, edge SubedgeID) error {
	edgeOwner, ok := c.SubedgeOwner(edge)
	if !ok || edgeOwner != owner {
		return errors.New("target: malformed subedge owner")
	}
	role, ok := c.SubedgeRole(edge)
	if !ok || role == 0 {
		return errors.New("target: malformed subedge role")
	}
	family, ok := c.SubedgeFamily(edge)
	if !ok || !validSubedgeFamily(family) {
		return errors.New("target: malformed subedge family")
	}
	callee, ok := c.SubedgeCallee(edge)
	if !ok {
		return errors.New("target: malformed subedge callee")
	}
	admission, ok := c.SubedgeAdmission(edge)
	if !ok || !validAdmission(admission) {
		return errors.New("target: malformed subedge admission")
	}
	arguments, ok := c.SubedgeArguments(edge)
	if !ok {
		return errors.New("target: malformed subedge arguments")
	}
	if err := w.Record(recordSubedge); err != nil {
		return err
	}
	if err := w.Uint(uint64(role)); err != nil {
		return err
	}
	if err := w.Uint(uint64(family)); err != nil {
		return err
	}
	if err := w.Uint(uint64(callee)); err != nil {
		return err
	}
	if err := w.Uint(uint64(admission)); err != nil {
		return err
	}
	switch callee {
	case SubedgeCalleeInvalid:
		if family == SubedgeFamilyCall {
			return errors.New("target: Call subedge lacks callee")
		}
	case SubedgeCalleeCallback:
		callback, found := c.SubedgeCallback(edge)
		if !found {
			return errors.New("target: malformed callback subedge callee")
		}
		if err := w.Uint(uint64(callback)); err != nil {
			return err
		}
	case SubedgeCalleeCapturedInitialRead:
		root, key, found := c.SubedgeCapturedInitialRead(edge)
		if !found {
			return errors.New("target: malformed captured initial read")
		}
		if err := w.Uint(uint64(root)); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	case SubedgeCalleeMetaKey:
		key, found := c.SubedgeMetaKey(edge)
		if !found {
			return errors.New("target: malformed metakey subedge callee")
		}
		if err := encodeExactKey(w, c, key); err != nil {
			return err
		}
	default:
		return errors.New("target: invalid subedge callee")
	}
	if err := encodeValues(w, c, arguments); err != nil {
		return err
	}
	ruleEntry, found := c.SubedgeRuleEntry(edge)
	if !found {
		return errors.New("target: malformed subedge entry authority")
	}
	if err := w.Bool(ruleEntry); err != nil {
		return err
	}
	originCount := c.ArgumentOriginCount(edge)
	if err := w.Count(uint64(originCount)); err != nil {
		return err
	}
	for index := 0; index < originCount; index++ {
		segment, ordinal, source, input, found := c.ArgumentOriginAt(edge, index)
		if !found || segment == ArgumentSegmentInvalid || source == ArgumentSourceInvalid {
			return errors.New("target: malformed subedge argument origin")
		}
		if err := encodeCoordinate(w, uint64(segment), uint64(ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(source)); err != nil {
			return err
		}
		if source == ArgumentSourceInput {
			if err := encodeCoordinate(w, uint64(input.Kind), uint64(input.Ordinal)); err != nil {
				return err
			}
		} else if input != (InputSource{}) {
			return errors.New("target: Rule argument origin carries input")
		}
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		terminal, found := c.SubedgeTerminal(edge, kind)
		if !found {
			return errors.New("target: malformed subedge terminal")
		}
		if err := w.Uint(uint64(kind)); err != nil {
			return err
		}
		if err := encodeValues(w, c, terminal); err != nil {
			return err
		}
	}
	failure, found := c.AdmissionFailure(edge)
	if !found {
		return errors.New("target: malformed subedge admission failure")
	}
	if err := encodeValues(w, c, failure); err != nil {
		return err
	}
	route, adjustment, result, placement, offset, outcome, sibling, destination, found := c.AdmissionRoute(edge)
	if !found || route == RouteInvalid {
		return errors.New("target: malformed subedge admission route")
	}
	if err := encodeSubedgeRoute(w, c, owner, route, adjustment, result, placement, offset, outcome, sibling, destination); err != nil {
		return err
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		route, adjustment, result, placement, offset, outcome, sibling, destination, found := c.SubedgeRouteAt(edge, kind)
		if !found || route == RouteInvalid {
			return errors.New("target: malformed subedge route")
		}
		if err := w.Uint(uint64(kind)); err != nil {
			return err
		}
		if err := encodeSubedgeRoute(w, c, owner, route, adjustment, result, placement, offset, outcome, sibling, destination); err != nil {
			return err
		}
	}
	return nil
}

func encodeSubedgeRoute(w *framing.Writer, c *Contract, owner Operation, route SubedgeRoute, adjustment Adjustment, result Values, placement Placement, offset uint32, outcome uint32, sibling SubedgeID, destination Values) error {
	if err := w.Uint(uint64(route)); err != nil {
		return err
	}
	if err := w.Uint(uint64(adjustment)); err != nil {
		return err
	}
	if result == 0 {
		return errors.New("target: subedge route lacks Result")
	}
	if err := encodeValues(w, c, result); err != nil {
		return err
	}
	if err := w.Uint(uint64(placement)); err != nil {
		return err
	}
	if err := w.Uint(uint64(offset)); err != nil {
		return err
	}
	switch route {
	case RouteOutcome:
		if sibling != 0 || destination == 0 {
			return errors.New("target: malformed outcome subedge route")
		}
		if err := w.Uint(uint64(outcome)); err != nil {
			return err
		}
		return encodeValues(w, c, destination)
	case RouteRejectYield:
		if destination == 0 {
			return errors.New("target: malformed C-boundary subedge route")
		}
		if err := w.Bool(sibling != 0); err != nil {
			return err
		}
		if sibling == 0 {
			if err := w.Uint(uint64(outcome)); err != nil {
				return err
			}
		} else {
			if outcome != 0 {
				return errors.New("target: C-boundary sibling route carries outcome")
			}
			siblingOwner, ownerOK := c.SubedgeOwner(sibling)
			siblingRole, roleOK := c.SubedgeRole(sibling)
			if !ownerOK || siblingOwner != owner || !roleOK || siblingRole == 0 {
				return errors.New("target: malformed C-boundary sibling role")
			}
			if err := w.Uint(uint64(siblingRole)); err != nil {
				return err
			}
		}
		return encodeValues(w, c, destination)
	case RouteSubedge:
		if outcome != 0 || sibling == 0 || destination == 0 {
			return errors.New("target: malformed sibling subedge route")
		}
		siblingOwner, ownerOK := c.SubedgeOwner(sibling)
		siblingRole, roleOK := c.SubedgeRole(sibling)
		if !ownerOK || siblingOwner != owner || !roleOK || siblingRole == 0 {
			return errors.New("target: malformed sibling semantic role")
		}
		if err := w.Uint(uint64(siblingRole)); err != nil {
			return err
		}
		return encodeValues(w, c, destination)
	case RouteContinue, RoutePropagateYield:
		if outcome != 0 || sibling != 0 || destination != 0 {
			return errors.New("target: malformed terminal-only subedge route")
		}
		return nil
	default:
		return errors.New("target: invalid subedge route")
	}
}

func encodeValues(w *framing.Writer, c *Contract, values Values) error {
	if err := w.Record(recordValues); err != nil {
		return err
	}
	fixed := c.ValuesCount(values)
	if err := w.Count(uint64(fixed)); err != nil {
		return err
	}
	for index := 0; index < fixed; index++ {
		value, ok := c.ValuesAt(values, index)
		if !ok {
			return errors.New("target: malformed fixed Values type")
		}
		if err := encodeType(w, c, value); err != nil {
			return err
		}
	}
	tail, variable, ok := c.ValuesTail(values)
	if !ok {
		return errors.New("target: malformed Values tail")
	}
	if err := w.Uint(uint64(tail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(variable)); err != nil {
		return err
	}
	suffix := c.ValuesSuffixCount(values)
	if err := w.Count(uint64(suffix)); err != nil {
		return err
	}
	for index := 0; index < suffix; index++ {
		value, found := c.ValuesSuffixAt(values, index)
		if !found {
			return errors.New("target: malformed Values suffix type")
		}
		if err := encodeType(w, c, value); err != nil {
			return err
		}
	}
	return nil
}

func encodeType(w *framing.Writer, c *Contract, value Type) error {
	if value == 0 || uint64(value) > uint64(len(c.types)) {
		return errors.New("target: malformed frozen type")
	}
	declaration := c.types[uint32(value)-1].declaration
	if !declaration.Available() {
		return errors.New("target: unavailable neutral type declaration")
	}
	primitive, primitiveOK := declaration.Primitive()
	if err := w.Bool(primitiveOK); err != nil {
		return err
	}
	if primitiveOK {
		if err := w.Uint(uint64(primitive)); err != nil {
			return err
		}
	} else if err := w.Uint(0); err != nil {
		return err
	}
	if err := w.Uint(uint64(declaration.ExternalFormals())); err != nil {
		return err
	}
	return w.Bytes(declaration.Bytes())
}

func encodeOutcome(w *framing.Writer, c *Contract, op Operation, outcome int) error {
	if err := w.Record(recordOutcome); err != nil {
		return err
	}
	kind, values, ok := c.OutcomeAt(op, outcome)
	if !ok {
		return errors.New("target: malformed outcome")
	}
	if err := w.Uint(uint64(kind)); err != nil {
		return err
	}
	if err := encodeValues(w, c, values); err != nil {
		return err
	}

	produced := c.ProducedCount(op, outcome)
	if err := w.Count(uint64(produced)); err != nil {
		return err
	}
	for index := 0; index < produced; index++ {
		result, target, found := c.ProducedAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed produced operation")
		}
		if err := w.Record(recordProduced); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(target)); err != nil {
			return err
		}
		captures := c.ProducedCaptureCount(op, outcome, index)
		if err := w.Count(uint64(captures)); err != nil {
			return err
		}
		for capture := 0; capture < captures; capture++ {
			kind, ordinal, found := c.ProducedCaptureAt(op, outcome, index, capture)
			if !found {
				return errors.New("target: malformed produced capture")
			}
			if err := w.Record(recordCapture); err != nil {
				return err
			}
			if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
				return err
			}
		}
	}

	callbackResults := c.CallbackResultCount(op, outcome)
	if err := w.Count(uint64(callbackResults)); err != nil {
		return err
	}
	for index := 0; index < callbackResults; index++ {
		result, callback, found := c.CallbackResultAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed callback result")
		}
		if err := w.Record(recordCallbackResult); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(callback)); err != nil {
			return err
		}
	}
	aliases := c.ResultAliasCount(op, outcome)
	if err := w.Count(uint64(aliases)); err != nil {
		return err
	}
	for index := 0; index < aliases; index++ {
		result, kind, ordinal, found := c.ResultAliasAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed result alias")
		}
		if err := w.Record(recordResultAlias); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
			return err
		}
	}
	fresh := c.FreshResultCount(op, outcome)
	if err := w.Count(uint64(fresh)); err != nil {
		return err
	}
	for index := 0; index < fresh; index++ {
		result, ordinal, kind, found := c.FreshResultAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed fresh result")
		}
		if err := w.Record(recordFreshResult); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(kind)); err != nil {
			return err
		}
	}
	return nil
}

func encodeEffect(w *framing.Writer, c *Contract, op Operation, effect int) error {
	row, ok := c.effect(op, effect)
	if !ok {
		return errors.New("target: malformed effect")
	}
	return encodeEffectRow(w, c, row)
}

func encodeEffectRow(w *framing.Writer, c *Contract, effect effectRow) error {
	if err := w.Record(recordEffect); err != nil {
		return err
	}
	if err := w.Uint(uint64(effect.target)); err != nil {
		return err
	}
	valueArgs := effect.values.len()
	if err := w.Count(uint64(valueArgs)); err != nil {
		return err
	}
	for index := 0; index < valueArgs; index++ {
		value := c.effectVals[effect.values.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	typeArgs := effect.types.len()
	if err := w.Count(uint64(typeArgs)); err != nil {
		return err
	}
	for index := 0; index < typeArgs; index++ {
		value := c.effectType[effect.types.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	valuesArgs := effect.valuesVar.len()
	if err := w.Count(uint64(valuesArgs)); err != nil {
		return err
	}
	for index := 0; index < valuesArgs; index++ {
		value := c.effectVars[effect.valuesVar.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	rowArgs := effect.rows.len()
	if err := w.Count(uint64(rowArgs)); err != nil {
		return err
	}
	for index := 0; index < rowArgs; index++ {
		value := c.effectRows[effect.rows.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	if err := w.Bool(effect.hasPublication); err != nil {
		return err
	}
	if effect.hasPublication {
		if !c.validPublicationEffectRow(effect) {
			return errors.New("target: malformed publication effect selector")
		}
		if err := encodePublicationEffectDescriptor(w, effect.publication); err != nil {
			return err
		}
	}
	return nil
}

func encodePublicationEffectDescriptor(w *framing.Writer, descriptor PublicationEffectDescriptor) error {
	if !descriptor.validConsequences() {
		return errors.New("target: malformed publication effect descriptor")
	}
	if err := w.Uint(uint64(descriptor.kind)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.subject)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.destination)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.context)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.escape)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.mutability)); err != nil {
		return err
	}
	return w.Uint(uint64(descriptor.lifetime))
}

func encodeProtocol(w *framing.Writer, c *Contract, protocol Protocol) error {
	if err := w.Record(recordProtocol); err != nil {
		return err
	}
	if err := w.Uint(uint64(protocol)); err != nil {
		return err
	}
	states := c.StateCount(protocol)
	if err := w.Count(uint64(states)); err != nil {
		return err
	}
	for index := 0; index < states; index++ {
		state, ok := c.StateAt(protocol, index)
		if !ok {
			return errors.New("target: malformed protocol state")
		}
		final, found := c.StateFinal(protocol, state)
		if !found {
			return errors.New("target: malformed state finality")
		}
		if err := w.Record(recordState); err != nil {
			return err
		}
		if err := w.Bool(final); err != nil {
			return err
		}
	}
	acquisitions := c.ProtocolAcquisitionCount(protocol)
	if err := w.Count(uint64(acquisitions)); err != nil {
		return err
	}
	for index := 0; index < acquisitions; index++ {
		op, outcome, result, state, ok := c.ProtocolAcquisitionAt(protocol, index)
		if !ok {
			return errors.New("target: malformed acquisition")
		}
		if err := w.Record(recordAcquisition); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := w.Uint(uint64(outcome)); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(state)); err != nil {
			return err
		}
	}
	transitions := c.TransitionCount(protocol)
	if err := w.Count(uint64(transitions)); err != nil {
		return err
	}
	for index := 0; index < transitions; index++ {
		op, kind, ordinal, from, ok := c.TransitionAt(protocol, index)
		if !ok {
			return errors.New("target: malformed transition")
		}
		if err := w.Record(recordTransition); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(from)); err != nil {
			return err
		}
		outcomes := c.TransitionOutcomeCount(protocol, index)
		if err := w.Count(uint64(outcomes)); err != nil {
			return err
		}
		for outcomeIndex := 0; outcomeIndex < outcomes; outcomeIndex++ {
			outcome, to, found := c.TransitionOutcomeAt(protocol, index, outcomeIndex)
			if !found {
				return errors.New("target: malformed transition outcome")
			}
			if err := w.Record(recordTransitionOutcome); err != nil {
				return err
			}
			if err := w.Uint(uint64(outcome)); err != nil {
				return err
			}
			if err := w.Uint(uint64(to)); err != nil {
				return err
			}
		}
	}
	escapes := c.EscapeCount(protocol)
	if err := w.Count(uint64(escapes)); err != nil {
		return err
	}
	for index := 0; index < escapes; index++ {
		op, kind, ordinal, ok := c.EscapeAt(protocol, index)
		if !ok {
			return errors.New("target: malformed escape")
		}
		if err := w.Record(recordEscape); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
			return err
		}
	}
	holders := c.ProtocolCallbackHolderCount(protocol)
	if err := w.Count(uint64(holders)); err != nil {
		return err
	}
	for index := 0; index < holders; index++ {
		op, input, callback, ok := c.ProtocolCallbackHolderAt(protocol, index)
		if !ok {
			return errors.New("target: malformed protocol callback holder")
		}
		if err := w.Record(recordProtocolCallbackHolder); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(input.Kind), uint64(input.Ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(callback)); err != nil {
			return err
		}
	}
	return nil
}

func encodeCoordinate(w *framing.Writer, kind, ordinal uint64) error {
	if err := w.Uint(kind); err != nil {
		return err
	}
	return w.Uint(ordinal)
}
