package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ResolvedPathStoreWrite is one value/path tuple whose sources were resolved
// before State mutation. It is shared by concrete and symbolic callers.
type ResolvedPathStoreWrite struct {
	Target         pathdom.Path
	Value          product.Value
	SourcePath     pathdom.Path
	HasSourcePath  bool
	SuppressProof  bool
	SourceStateKey pathaddr.StateKey
	HasSourceState bool
	Expected       product.Value
	HasExpected    bool
}

type ResolvedPathStoreHeapMember struct {
	Suffix      []segment.Segment
	Value       product.Value
	Expected    product.Value
	HasExpected bool
}

// ResolvedPathStoreHeapObject is post-ordered: nested objects precede owners.
type ResolvedPathStoreHeapObject struct {
	Root        product.Value
	Members     []ResolvedPathStoreHeapMember
	StableShape bool
}

type ResolvedPathStoreObject struct {
	Heaps     []ResolvedPathStoreHeapObject
	Entries   []ResolvedPathStoreWrite
	ListFloor int64
}

// ResolvedPathStoreTransaction is the sole N4 State program. It contains no
// Facts, ValueSource, source resolver, CFG traversal, or callback.
type ResolvedPathStoreTransaction struct {
	Point                 cfg.Point
	Assignment            ResolvedPathStoreWrite
	HasAssignment         bool
	Static                ResolvedPathStoreWrite
	HasStatic             bool
	StaticHasAnnotation   bool
	Object                ResolvedPathStoreObject
	PresencePublications  []pathevidence.PathPresenceImplication
	DynamicIndexSource    pathdom.Path
	HasDynamicIndexSource bool
}

// Clone returns a detached copy suitable for retention in a sealed program.
// In particular, object-entry paths and heap-member suffixes never alias
// compiler scratch or an executor-local overlay pass.
func (t ResolvedPathStoreTransaction) Clone() ResolvedPathStoreTransaction {
	t.Assignment = cloneResolvedPathStoreWrite(t.Assignment)
	t.Static = cloneResolvedPathStoreWrite(t.Static)
	t.DynamicIndexSource = t.DynamicIndexSource.Clone()
	t.PresencePublications = append([]pathevidence.PathPresenceImplication(nil), t.PresencePublications...)
	t.Object = cloneResolvedPathStoreObject(t.Object)
	return t
}

func cloneResolvedPathStoreObject(object ResolvedPathStoreObject) ResolvedPathStoreObject {
	object.Entries = append([]ResolvedPathStoreWrite(nil), object.Entries...)
	for index := range object.Entries {
		object.Entries[index] = cloneResolvedPathStoreWrite(object.Entries[index])
	}
	object.Heaps = append([]ResolvedPathStoreHeapObject(nil), object.Heaps...)
	for heapIndex := range object.Heaps {
		heap := &object.Heaps[heapIndex]
		heap.Members = append([]ResolvedPathStoreHeapMember(nil), heap.Members...)
		for memberIndex := range heap.Members {
			heap.Members[memberIndex].Suffix = append([]segment.Segment(nil), heap.Members[memberIndex].Suffix...)
		}
	}
	return object
}

func cloneResolvedPathStoreWrite(write ResolvedPathStoreWrite) ResolvedPathStoreWrite {
	write.Target = write.Target.Clone()
	write.SourcePath = write.SourcePath.Clone()
	return write
}

func (t ResolvedPathStoreTransaction) HasPathAssignment() bool    { return t.HasAssignment }
func (t ResolvedPathStoreTransaction) HasStaticMemberWrite() bool { return t.HasStatic }
func (t ResolvedPathStoreTransaction) HasObjectLiteral() bool {
	return len(t.Object.Heaps) != 0 || len(t.Object.Entries) != 0 || t.Object.ListFloor != 0
}
func (t ResolvedPathStoreTransaction) HasStateSteps() bool {
	return t.HasAssignment || t.HasStatic || len(t.PresencePublications) != 0
}

func (t ResolvedPathStoreTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil || t.Point <= 0 || !t.HasStateSteps() {
		return false
	}
	hasObject := len(t.Object.Heaps) != 0 || len(t.Object.Entries) != 0 || t.Object.ListFloor != 0
	if t.Object.ListFloor < 0 || hasObject && !t.HasAssignment {
		return false
	}
	validWrite := func(write ResolvedPathStoreWrite) bool {
		return !write.Target.IsEmpty() && write.Target.Symbol != 0 && len(write.Target.Segments) != 0 &&
			write.HasSourcePath == (!write.SourcePath.IsEmpty() && write.SourcePath.Symbol != 0)
	}
	if t.HasAssignment && !validWrite(t.Assignment) || t.HasStatic && !validWrite(t.Static) {
		return false
	}
	validValue := func(value product.Value) bool { return product.BelongsToRegistry(reg, value) }
	if t.HasAssignment && (!validValue(t.Assignment.Value) || t.Assignment.HasExpected && !validValue(t.Assignment.Expected)) || t.HasStatic && (!validValue(t.Static.Value) || t.Static.HasExpected && !validValue(t.Static.Expected)) {
		return false
	}
	for _, entry := range t.Object.Entries {
		if !validWrite(entry) || !validValue(entry.Value) || entry.HasExpected && !validValue(entry.Expected) {
			return false
		}
	}
	for _, object := range t.Object.Heaps {
		if !validValue(object.Root) {
			return false
		}
		for _, member := range object.Members {
			if len(member.Suffix) == 0 || !validValue(member.Value) || member.HasExpected && !validValue(member.Expected) {
				return false
			}
		}
	}
	for _, implication := range t.PresencePublications {
		if implication.HasTriggerValue && !validValue(implication.TriggerValue) ||
			implication.HasTargetValue && !validValue(implication.TargetValue) {
			return false
		}
	}
	return true
}

type ResolvedPathStoreRequest struct {
	Context     transfer.NodeContext
	Resolver    *visibility.Resolver
	Input       state.State
	Output      state.State
	Transaction ResolvedPathStoreTransaction
}

type ResolvedPathStoreResult struct {
	Output            state.State
	AssignmentApplied bool
	Canceled          bool
}

// ApplyResolvedPathStore is the one concrete N4 executor.
func ApplyResolvedPathStore(req ResolvedPathStoreRequest) ResolvedPathStoreResult {
	t := req.Transaction
	if req.Resolver == nil || req.Context.Registry == nil || req.Context.Point != t.Point || !t.Valid(req.Context.Registry) {
		return ResolvedPathStoreResult{Output: req.Output}
	}
	token := tokenOf(req.Context.Session)
	canceled := func() ResolvedPathStoreResult { return ResolvedPathStoreResult{Output: req.Input, Canceled: true} }
	if token != nil && token.Canceled() {
		return canceled()
	}
	out, applied := req.Output, false
	if t.HasAssignment {
		out, applied = applyResolvedPathAssignment(req, out, t.Assignment, t.HasStatic && t.Static.Target.Equal(t.Assignment.Target))
		if token != nil && token.Canceled() {
			return canceled()
		}
		if applied {
			out = applyResolvedPathStoreObject(req, out, t.Object)
		}
	}
	if len(t.PresencePublications) != 0 {
		presence := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{Registry: req.Context.Registry, Resolver: req.Resolver, Point: t.Point, Output: out, Publications: t.PresencePublications, Token: token})
		if presence.Canceled {
			return canceled()
		}
		out = presence.Output
	}
	if t.HasStatic {
		out = applyResolvedPathStatic(req, out, t)
	}
	if token != nil && token.Canceled() {
		return canceled()
	}
	return ResolvedPathStoreResult{Output: out, AssignmentApplied: applied}
}

func applyResolvedPathAssignment(req ResolvedPathStoreRequest, out state.State, write ResolvedPathStoreWrite, pairedStatic bool) (state.State, bool) {
	keys := req.Resolver.KeySpace()
	target, targetOK := visibility.AddressAt(req.Resolver, req.Context.Point, write.Target).VisibleLocalKeyspaceKey()
	if !targetOK {
		return out, false
	}
	config := state.PathReplacementConfig{Keys: keys, Target: target, Value: write.Value, PairedStatic: pairedStatic}
	if write.HasSourcePath && !write.SuppressProof {
		config.Source, config.HasSource = visibility.AddressAt(req.Resolver, req.Context.Point, write.SourcePath).VisibleLocalKeyspaceKey()
		if !config.HasSource {
			return out, false
		}
	}
	if write.HasSourceState {
		config.UserSource, config.HasUserSource = keys.InternStateKey(write.SourceStateKey)
		if !config.HasUserSource {
			return out, false
		}
	}
	domain := state.RegisteredProductDomain(req.Context.Registry)
	written, applied, err := domain.ApplyConcretePathReplacement(config, req.Input, out)
	if err != nil || !applied {
		return out, false
	}
	return written, true
}

func addResolvedPathEquality(reg *axis.Registry, resolver *visibility.Resolver, point cfg.Point, out state.State, write ResolvedPathStoreWrite) state.State {
	if !write.HasSourcePath || write.SuppressProof || write.SourcePath.IsEmpty() || write.SourcePath.Symbol == 0 {
		return out
	}
	return addPathEqualityProofAt(reg, resolver, point, out, write.Target, write.SourcePath)
}

func applyResolvedPathStoreObject(req ResolvedPathStoreRequest, out state.State, object ResolvedPathStoreObject) state.State {
	ks := req.Resolver.KeySpace()
	out = applyResolvedObjectHeaps(req.Context.Registry, ks, out, object.Heaps)
	if object.ListFloor > 0 && req.Transaction.HasAssignment {
		if target, ok := visibility.AddressAt(req.Resolver, req.Context.Point, req.Transaction.Assignment.Target).VisibleStateKey(); ok {
			if path, interned := ks.InternStateKey(target); interned {
				domain := state.RegisteredProductDomain(req.Context.Registry)
				if plan, planErr := domain.PrepareLengthFloorFactorPlan(ks, path, object.ListFloor); planErr == nil {
					if written, applyErr := domain.ApplyLengthFloor(plan, out); applyErr == nil {
						out = written
					}
				}
			}
		}
	}
	for _, entry := range object.Entries {
		value := entry.Value
		if req.Transaction.HasAssignment {
			if rootEvidence, ok := untrustedRootEvidence(req.Context.Registry, out, req.Transaction.Assignment.Target.Symbol); ok {
				value = product.Set(req.Context.Registry, value, evidence.Key, rootEvidence)
			}
		}
		invalidated, ok := invalidatePathSubtreeAt(out, req.Resolver, req.Context.Point, entry.Target)
		if !ok {
			continue
		}
		written, ok := writePathAt(req.Context.Registry, invalidated, req.Resolver, req.Context.Point, entry.Target, value)
		if ok {
			entry.Value = value
			out = addResolvedPathEquality(req.Context.Registry, req.Resolver, req.Context.Point, written, entry)
		}
	}
	return out
}

func applyResolvedObjectHeaps(reg *axis.Registry, ks *keyspace.KeySpace, out state.State, heaps []ResolvedPathStoreHeapObject) state.State {
	result, err := applyResolvedObjectHeapsChecked(reg, ks, out, heaps)
	if err != nil {
		return out
	}
	return result
}

func applyResolvedObjectHeapsChecked(reg *axis.Registry, ks *keyspace.KeySpace, out state.State, heaps []ResolvedPathStoreHeapObject) (state.State, error) {
	shapes := make([]state.ObjectConstructorShape, 0, len(heaps))
	values := make([]state.ObjectConstructorValues, 0, len(heaps))
	for _, heap := range heaps {
		id, ok := product.Get(reg, heap.Root, identity.Key).ID()
		if !ok {
			continue
		}
		shape := state.ObjectConstructorShape{Identity: identity.ConcreteTerm(id), StableShape: heap.StableShape}
		row := state.ObjectConstructorValues{Root: heap.Root}
		for _, member := range heap.Members {
			if _, ok := ks.FromRootlessSuffix(member.Suffix); !ok {
				continue
			}
			shape.MemberSuffixes = append(shape.MemberSuffixes, member.Suffix)
			row.Members = append(row.Members, member.Value)
		}
		shapes = append(shapes, shape)
		values = append(values, row)
	}
	if len(shapes) == 0 {
		return out, nil
	}
	domain := state.RegisteredProductDomain(reg)
	plan, err := domain.PrepareObjectConstructorPlan(ks, shapes)
	if err != nil {
		return state.State{}, err
	}
	result, err := domain.ApplyObjectConstructor(plan, values, out)
	if err != nil {
		return state.State{}, err
	}
	return result, nil
}

func applyResolvedPathStatic(req ResolvedPathStoreRequest, out state.State, transaction ResolvedPathStoreTransaction) state.State {
	write := transaction.Static
	targetKey := factPathKeyAt(req.Resolver, req.Context.Point, write.Target)
	if targetKey == "" {
		return out
	}
	ks := req.Resolver.KeySpace()
	local, ok := ks.FromPathKey(targetKey)
	if !ok {
		return out
	}
	domain := state.RegisteredProductDomain(req.Context.Registry)
	plan, err := domain.PrepareStaticMemberFactorPlan(ks, local, write.Value)
	if err != nil {
		return out
	}
	written, err := domain.ApplyStaticMember(plan, out)
	if err != nil {
		return out
	}
	out = written
	out = applyPathStaticMemberWriteContainerPresence(req.Context, req.Resolver, out, write.Target)
	out = writeHeapTableStaticMember(req.Context, req.Resolver, out, write.Target, write.Value)
	out = applyStoredStaticMemberPlacement(req.Context, req.Resolver, out, write.Target, write.Value)
	out = addResolvedPathEquality(req.Context.Registry, req.Resolver, req.Context.Point, out, write)
	if transaction.HasDynamicIndexSource {
		out = addPathEqualityProofAt(req.Context.Registry, req.Resolver, req.Context.Point, out, write.Target, transaction.DynamicIndexSource)
	}
	return out
}
