package engine

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/module/exportrelation"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

// PlacementPlan is the public, read-only projection of placement facts closed
// by one engine evaluation. It is intentionally absent when no allocation
// fact was established; nil is not an optimistic empty plan.
type PlacementPlan struct {
	Complete       bool
	Allocations    []PlacementAllocation
	HoistableLoads []PlacementHoistableLoad
}

// PlacementAllocation contains only conclusions carried by closure facts.
// Placement is Unknown whenever a retaining boundary was not proven safe.
type PlacementAllocation struct {
	Identity                string
	Target                  string
	Kind                    string
	Placement               placement.Value
	Complete                bool
	Blockers                []string
	Depth                   int
	Decomposable            bool
	FrameLocal              bool
	DiesBeforeSuspension    bool
	HasDiesBeforeSuspension bool
	OwnerIdentity           bool
	SealBeforeShare         bool
	Obligations             []string
}

// PlacementHoistableLoad is deliberately sparse: the plan reports a load only
// when a kernel has established every required motion proof.
type PlacementHoistableLoad struct {
	Target string
}

const (
	placementAllocationPrefix  = "placement/allocation/"
	placementBindingPrefix     = "placement/binding/"
	placementEventPrefix       = "placement/event/"
	placementBlockerPrefix     = "placement/blocker/"
	placementContainmentPrefix = "placement/contains/"
	placementContractPrefix    = "placement/contract/"

	// placementLocalReturnRootPrefix carries the returned-root allocation identity
	// of a locally evaluated body from its apply boundary to the call-results
	// owner, which binds the caller result to it.
	placementLocalReturnRootPrefix = "placement/local-return-root/"

	placementEventOwned  = "owned"
	placementEventShared = "shared"
	placementEventSealed = "sealed"
	// A suspension event is lifetime evidence, not an ownership transfer. The
	// allocation remains stack-placed, but neither frame-local license holds.
	placementEventSuspended = "suspended"
)

type placementAllocationFact struct {
	Identity     string   `json:"identity"`
	Result       string   `json:"result"`
	Kind         string   `json:"kind"`
	Complete     bool     `json:"complete"`
	Decomposable bool     `json:"decomposable"`
	Children     []string `json:"children,omitempty"`
}

// placementClosedAllocation is the narrow admission boundary shared by the
// local operation witnesses below.  It is deliberately about the published
// allocation identity, not the spelling of the source term: an incomplete,
// non-decomposable, or open table remains subject to the ordinary blocker.
func placementClosedAllocation(allocation placementAllocationFact, partition equation.Partition) bool {
	if !allocation.Complete || !allocation.Decomposable {
		return false
	}
	identity, found := placementAllocationIdentityBytes(allocation, partition)
	return found && heapTableClosed(identity, partition) && !heapMetaAttached(identity, partition) && !heapHasExternalCallback(identity, partition)
}

func placementAllocationIdentityBytes(allocation placementAllocationFact, partition equation.Partition) ([]byte, bool) {
	if allocation.Result == "" {
		return nil, false
	}
	return tableIdentityForTerm([]byte(allocation.Result), partition)
}

func encodePlacementAllocation(fact placementAllocationFact) ([]byte, error) {
	return json.Marshal(fact)
}

func decodePlacementAllocation(fact equation.Fact) (placementAllocationFact, bool) {
	var allocation placementAllocationFact
	return allocation, strings.HasPrefix(fact.Key, placementAllocationPrefix) && json.Unmarshal(fact.Value, &allocation) == nil && allocation.Identity != ""
}

func placementProvenFactParts(fact equation.Fact) ([]string, bool) {
	parts := strings.Split(fact.Key, "/")
	return parts, len(parts) == 5 && string(fact.Value) == "proven"
}

func placementFactIdentity(parts []string) (string, bool) {
	identity, err := base64.RawURLEncoding.DecodeString(parts[2])
	return string(identity), err == nil
}

func placementContainmentIdentities(parts []string) (string, string, bool) {
	parent, parentErr := base64.RawURLEncoding.DecodeString(parts[2])
	child, childErr := base64.RawURLEncoding.DecodeString(parts[3])
	return string(parent), string(child), parentErr == nil && childErr == nil
}

// placementAllocationFactKey is identity-addressed so facts from an evaluated
// lexical child can cross its publication boundary without colliding with a
// same-named operation in its caller. The identity itself is sealed from the
// child body and allocation occurrence.
func placementAllocationFactKey(identity string) string {
	return placementAllocationPrefix + base64.RawURLEncoding.EncodeToString([]byte(identity))
}

func placementAllocationIdentity(operation equation.BoundEquation) string {
	return "allocation/" + string(operation.Target.Body[:]) + "/" + operation.Target.Name
}

func placementBindingFact(term, operation, identity string) equation.Fact {
	return equation.Fact{Key: placementBindingPrefix + base64.RawURLEncoding.EncodeToString([]byte(term)) + "/" + operation, Value: []byte(identity)}
}

func placementEventFact(identity, operation, event string) equation.Fact {
	return equation.Fact{Key: placementEventPrefix + base64.RawURLEncoding.EncodeToString([]byte(identity)) + "/" + event + "/" + operation, Value: []byte("proven")}
}

func placementBlockerFact(identity, operation, blocker string) equation.Fact {
	return equation.Fact{Key: placementBlockerPrefix + base64.RawURLEncoding.EncodeToString([]byte(identity)) + "/" + blocker + "/" + operation, Value: []byte("proven")}
}

func placementContainmentFact(parent, child, operation string) equation.Fact {
	return equation.Fact{Key: placementContainmentPrefix + base64.RawURLEncoding.EncodeToString([]byte(parent)) + "/" + base64.RawURLEncoding.EncodeToString([]byte(child)) + "/" + operation, Value: []byte("proven")}
}

func placementContractFact(identity, boundary, operation string) equation.Fact {
	return equation.Fact{Key: placementContractPrefix + base64.RawURLEncoding.EncodeToString([]byte(identity)) + "/" + boundary + "/" + operation, Value: []byte("proven")}
}

// placementExternalOwnershipFacts consumes retaining ownership labels from the
// published provider signature. The external-call factor has the exact
// provider identity and matching apply coordinate, so it can discharge only
// the opaque-call fallback emitted for that same application. Unknown
// providers, unlabelled calls, and non-retaining labels stay blocked.
func placementExternalOwnershipFacts(operation equation.BoundEquation, provider []byte, arguments [][]byte, partition equation.Partition) []equation.Fact {
	name, found := placementGlobalProviderName(provider)
	if !found {
		return nil
	}
	signature, found := (signaturelookup.Source{IncludeStdlib: true}).LookupView(name)
	if !found || !signature.Effect.IsClosed() {
		return nil
	}
	application := strings.TrimPrefix(string(operationOperandValue(operation, "application")), "call/")
	if application == "" {
		return nil
	}
	var facts []equation.Fact
	for _, label := range signature.Effect.Labels {
		var from int
		boundary, event := "", ""
		switch value := effect.NormalizeLabel(label).(type) {
		case ownership.Send:
			from = value.FromParam
			boundary, event = "send", placementEventShared
		case ownership.SendParam:
			var resolved bool
			from, resolved = effect.ResolveParamIndex(value.Param, len(arguments))
			if !resolved {
				continue
			}
			boundary, event = "send", placementEventShared
		case ownership.Retain:
			var resolved bool
			from, resolved = effect.ResolveParamIndex(value.Param, len(arguments))
			if !resolved {
				continue
			}
			boundary, event = "retain", placementEventOwned
		default:
			continue
		}
		if from < 0 || from >= len(arguments) {
			continue
		}
		for index := from; index < len(arguments); index++ {
			if boundary == "retain" && index != from {
				break
			}
			allocation, exists := placementAllocationForTerm(arguments[index], partition)
			if !exists {
				continue
			}
			facts = append(facts,
				placementEventFact(allocation.Identity, application, event),
				placementContractFact(allocation.Identity, boundary, application),
			)
		}
	}
	return facts
}

func placementGlobalProviderName(provider []byte) (string, bool) {
	encoded := strings.TrimPrefix(string(provider), "provider/global/")
	if encoded == string(provider) || encoded == "" {
		return "", false
	}
	name, err := strconv.Unquote(encoded)
	return name, err == nil && name != ""
}

func operationOperandValue(operation equation.BoundEquation, role string) []byte {
	for _, operand := range operation.Operands {
		if operand.Role == role {
			return operand.Value
		}
	}
	return nil
}

func placementExternalArguments(operation equation.BoundEquation) ([][]byte, bool) {
	indexed := make(map[int][]byte)
	for _, operand := range operation.Operands {
		if !strings.HasPrefix(operand.Role, "argument-") || operand.Role == "argument-spread" || strings.HasPrefix(operand.Role, "argument-display-") {
			continue
		}
		index, err := callArgumentIndex(operand.Role)
		if err != nil || indexed[index] != nil {
			return nil, false
		}
		indexed[index] = operand.Value
	}
	arguments := make([][]byte, len(indexed))
	for index := range arguments {
		if arguments[index] = indexed[index]; arguments[index] == nil {
			return nil, false
		}
	}
	return arguments, true
}

func placementAllocationForTerm(term []byte, partition equation.Partition) (placementAllocationFact, bool) {
	if len(term) == 0 {
		return placementAllocationFact{}, false
	}
	if identity, found := placementBindingForTerm(string(term), partition); found {
		return placementAllocation(identity, "", partition)
	}
	return placementAllocation("", string(term), partition)
}

func placementAllocation(identity, result string, partition equation.Partition) (placementAllocationFact, bool) {
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, placementAllocationPrefix) {
			continue
		}
		var allocation placementAllocationFact
		if json.Unmarshal(fact.Value, &allocation) == nil && allocation.Identity != "" &&
			(identity != "" && allocation.Identity == identity || result != "" && allocation.Result == result) {
			return allocation, true
		}
	}
	return placementAllocationFact{}, false
}

// placementImportedReturnFacts instantiates the finite table graph carried by
// an exact imported return relation. The relation is admitted only after the
// module exporter has validated its callable surface, and call-results has
// matched it to this provider and return slot. These are fresh allocation
// witnesses at this call boundary, not aliases reconstructed from a result
// type or source spelling.
func placementImportedReturnFacts(template exportrelation.Value, result, application string) []equation.Fact {
	if result == "" || application == "" || len(template.Table) == 0 {
		return nil
	}
	var facts []equation.Fact
	var instantiate func(exportrelation.Value, string, string) string
	instantiate = func(value exportrelation.Value, target, suffix string) string {
		if len(value.Table) == 0 {
			return ""
		}
		identity := "allocation/imported/" + base64.RawURLEncoding.EncodeToString([]byte(application)) + "/" + base64.RawURLEncoding.EncodeToString([]byte(suffix))
		children := make([]string, 0)
		for _, member := range value.Table {
			if len(member.Value.Table) == 0 {
				continue
			}
			childSuffix := suffix + member.Suffix
			childTarget := "placement/imported/" + base64.RawURLEncoding.EncodeToString([]byte(identity+"/"+member.Suffix))
			if instantiate(member.Value, childTarget, childSuffix) != "" {
				children = append(children, childTarget)
			}
		}
		encoded, err := encodePlacementAllocation(placementAllocationFact{
			Identity: identity, Result: target, Kind: "manifest.allocation", Complete: true, Children: children,
		})
		if err == nil {
			facts = append(facts,
				equation.Fact{Key: placementAllocationFactKey(identity), Value: encoded},
				// The imported relation is a fresh graph that has crossed its
				// producer's return boundary. Its exact call result is therefore
				// retained by this caller; it cannot remain frame-local even when
				// the caller only reads it. A later send may still promote this
				// same owned graph to shared.
				placementEventFact(identity, application, placementEventOwned),
			)
			return identity
		}
		return ""
	}
	instantiate(template, result, "root")
	return facts
}

// placementReturnedRoot names the root a locally evaluated body returns: the
// single allocation for which the body published an owned return-escape event.
// A body returns one root table, so a unique owned event identifies it; zero or
// several owned events leave the root ambiguous and the caller binding withheld.
func placementReturnedRoot(facts []equation.Fact) (string, bool) {
	allocations := make(map[string]bool)
	for _, fact := range facts {
		if allocation, ok := decodePlacementAllocation(fact); ok && allocation.Result != "" && allocation.Kind != "" {
			allocations[allocation.Identity] = true
		}
	}
	root, count := "", 0
	seen := make(map[string]bool)
	for _, fact := range facts {
		if !strings.HasPrefix(fact.Key, placementEventPrefix) {
			continue
		}
		parts, ok := placementProvenFactParts(fact)
		if !ok || parts[3] != placementEventOwned {
			continue
		}
		identity, ok := placementFactIdentity(parts)
		if !ok || !allocations[identity] || seen[identity] {
			continue
		}
		seen[identity] = true
		root, count = identity, count+1
	}
	if count != 1 {
		return "", false
	}
	return root, true
}

// placementMemberDescent resolves a member path over the published placement
// containment graph when the term has no direct binding of its own. A returned
// graph crosses its call boundary keyed by allocation identity, not by the
// caller's member spelling, so a read or send of a member reaches its allocation
// only by descent from the bound root. Each field or exact-index segment
// descends to the unique table child of the current allocation; a step with zero
// or several table children is ambiguous and fails closed, so no member acquires
// a placement it cannot uniquely justify.
func placementMemberDescent(term []byte, partition equation.Partition) (placementAllocationFact, bool) {
	root, suffix, ok := tableAddress(term)
	if !ok || suffix == "" || !placementDescentTargetsTable(term, partition) {
		return placementAllocationFact{}, false
	}
	current, found := placementAllocationForTerm(root, partition)
	if !found {
		return placementAllocationFact{}, false
	}
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid || len(segments) == 0 {
		return placementAllocationFact{}, false
	}
	parsed := parsePublishedPlacement(partition.Values())
	children := propagatePublishedPlacement(parsed)
	for range segments {
		next, ok := placementSingleTableChild(current.Identity, parsed, children)
		if !ok {
			return placementAllocationFact{}, false
		}
		current = next
	}
	return current, true
}

// placementSingleTableChild returns the unique table-shaped child of an
// allocation. A representative map value and a single named table field both
// resolve here; a scalar-only container, or one with several table children,
// stays ambiguous.
func placementSingleTableChild(identity string, parsed publishedPlacementFacts, children map[string][]string) (placementAllocationFact, bool) {
	var found placementAllocationFact
	count := 0
	seen := make(map[string]bool)
	for _, child := range children[identity] {
		if seen[child] {
			continue
		}
		seen[child] = true
		allocation, ok := parsed.allocations[child]
		if !ok || (allocation.Kind != "lua.table" && allocation.Kind != "manifest.allocation") {
			continue
		}
		found, count = allocation, count+1
	}
	if count != 1 {
		return placementAllocationFact{}, false
	}
	return found, true
}

func placementBindingForTerm(term string, partition equation.Partition) (string, bool) {
	prefix := placementBindingPrefix + base64.RawURLEncoding.EncodeToString([]byte(term)) + "/"
	latest, identity := "", ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && fact.Key > latest && len(fact.Value) != 0 {
			latest, identity = fact.Key, string(fact.Value)
		}
	}
	return identity, identity != ""
}

// placementApplyFacts marks only named retaining boundaries. Every other call
// carrying an allocation is a blocker, because opaque call summaries cannot
// certify that the allocation remains frame-local.
func placementApplyFacts(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) []equation.Fact {
	if operands.spread {
		return nil
	}
	var facts []equation.Fact
	for index, argument := range operands.arguments {
		allocation, found := placementAllocationForTerm(argument, partition)
		if !found {
			// A member of a locally returned graph has no binding of its own; it
			// reaches its allocation only by descent from the bound root.
			allocation, found = placementMemberDescent(argument, partition)
		}
		if !found {
			continue
		}
		switch {
		case operands.display == "ownership.store" && (index == 0 || index == 1):
			// The ownership contract names both arguments, so neither needs the
			// opaque fallback. Only the stored graph is retained past the caller
			// frame; the owner merely receives it and keeps whatever placement
			// its own allocation proves. This is the same split the imported
			// wrapper relation publishes for a declared store.
			facts = append(facts, placementContractFact(allocation.Identity, "retain", operation.Target.Name))
			if index == 0 {
				facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventOwned))
			}
		case (operands.display == "process.send" && index == 2) || (operands.method == "send" && index == 0 && typedChannelReceiver(operands.receiver, partition)):
			// The send boundary is the sealing event for its closed transfer
			// payload. Both conclusions are emitted from the same proved call
			// contract, so a later source mutation cannot masquerade as a
			// pre-send sealing proof.
			facts = append(facts,
				placementEventFact(allocation.Identity, operation.Target.Name, placementEventSealed),
				placementEventFact(allocation.Identity, operation.Target.Name, placementEventShared),
			)
		case operands.display == "table.freeze" && index == 0:
			facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventSealed))
		case operands.display == "setmetatable" && closedEmptyMetatable(operands.arguments, partition):
			// An exact closed table with an exact empty metatable has no callable
			// metamethod boundary.  Retain the ordinary opaque blocker for every
			// other metatable shape.
			facts = append(facts, placementContractFact(allocation.Identity, "metatable", operation.Target.Name))
		default:
			facts = append(facts, placementBlockerFact(allocation.Identity, operation.Target.Name, "opaque-call"))
		}
	}
	return facts
}

// placementSuspensionFacts promotes only allocation roots that the front has
// already proved live across a typed channel receive. The live-root operands
// are CFG-derived: a source allocation reaches this receive, has not been
// rebound, and has a reachable later read. A method spelling alone therefore
// cannot promote an arbitrary frame-local value.
func placementSuspensionFacts(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) []equation.Fact {
	if operands.method != "receive" || !typedChannelReceiver(operands.receiver, partition) {
		return nil
	}
	live := make(map[int][]byte)
	for _, operand := range operation.Operands {
		if !strings.HasPrefix(operand.Role, "suspension-live-") {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(operand.Role, "suspension-live-"))
		if err != nil || index < 0 || live[index] != nil {
			return nil
		}
		live[index] = operand.Value
	}
	facts := make([]equation.Fact, 0, len(live))
	for index := 0; index < len(live); index++ {
		term := live[index]
		if len(term) == 0 {
			return nil
		}
		allocation, found := placementAllocationForTerm(term, partition)
		if !found {
			return nil
		}
		facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventSuspended))
	}
	return facts
}

func closedEmptyMetatable(arguments [][]byte, partition equation.Partition) bool {
	if len(arguments) != 2 {
		return false
	}
	for _, argument := range arguments {
		allocation, found := placementAllocationForTerm(argument, partition)
		if !found || !placementClosedAllocation(allocation, partition) {
			return false
		}
	}
	value, err := resolveCurrentValue(arguments[1], partition)
	if err != nil {
		return false
	}
	table, ok := shapefact.DecodeTable(value)
	return ok && table.Closed && len(table.Members) == 0
}

// placementInvokedClosureCaptureFacts consumes the exact capture list of a
// lexical closure only at its established local invocation boundary. A merely
// materialized closure does not retain anything: the apply factor supplies the
// proof that this closed environment is live. Each captured allocation is
// looked up through its current published binding, so unrelated same-shaped or
// same-named values cannot acquire an ownership conclusion.
func placementInvokedClosureCaptureFacts(operation equation.BoundEquation, operands directCallOperands, handle closureHandle, partition equation.Partition) []equation.Fact {
	if len(handle.Captures) == 0 || operands.callee == nil {
		return nil
	}
	var facts []equation.Fact
	for _, capture := range handle.Captures {
		allocation, found := placementAllocationForTerm([]byte(capture), partition)
		if !found {
			continue
		}
		facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventOwned))
	}
	if len(facts) == 0 {
		return nil
	}
	if closure, found := placementAllocationForTerm(operands.callee, partition); found {
		facts = append(facts, placementEventFact(closure.Identity, operation.Target.Name, placementEventOwned))
	}
	return facts
}

// importedCallRelation resolves the published, checked wrapper relation for an
// imported provider call and the caller-visible application it discharges. No
// imported source spelling or arbitrary external callable resolves here.
func importedCallRelation(lexical *lexicalEvaluator, operation equation.BoundEquation, provider []byte, arity int) (exportrelation.Function, string, bool) {
	if lexical == nil {
		return exportrelation.Function{}, "", false
	}
	module, suffix, load, ok := importedProviderTarget(provider)
	if !ok || load || suffix == "" {
		return exportrelation.Function{}, "", false
	}
	lexical.importedAuthorityMu.RLock()
	summary, found := lexical.importedRelations[module]
	lexical.importedAuthorityMu.RUnlock()
	if !found {
		return exportrelation.Function{}, "", false
	}
	function, found := summary.Function(strings.TrimPrefix(suffix, "."), arity)
	if !found {
		return exportrelation.Function{}, "", false
	}
	application := strings.TrimPrefix(string(operationOperandValue(operation, "application")), "call/")
	if application == "" {
		return exportrelation.Function{}, "", false
	}
	return function, application, true
}

// placementImportedStoreFacts consumes an ownership boundary only when an
// imported module has already published a checked, one-statement store
// wrapper. A positional ownership.store names both the stored value and its
// owner; an escaping-root wrapper writes the value into module state and has no
// owner formal. Either way only the stored graph is retained; no imported
// source spelling or arbitrary external callable can retain a graph.
func placementImportedStoreFacts(lexical *lexicalEvaluator, operation equation.BoundEquation, provider []byte, arguments [][]byte, partition equation.Partition) []equation.Fact {
	function, application, ok := importedCallRelation(lexical, operation, provider, len(arguments))
	if !ok || !function.Store.Valid(len(arguments)) {
		return nil
	}
	indices := []int{function.Store.Value}
	if !function.Store.EscapingRoot {
		indices = append(indices, function.Store.Owner)
	}
	var facts []equation.Fact
	for _, index := range indices {
		allocation, found := placementAllocationForTerm(arguments[index], partition)
		if !found {
			continue
		}
		facts = append(facts, placementContractFact(allocation.Identity, "retain", application))
		if index == function.Store.Value {
			facts = append(facts, placementEventFact(allocation.Identity, application, placementEventOwned))
		}
	}
	return facts
}

// typedChannelReceiver admits the channel send placement boundary only when
// the receiver's payload contract has already been published. An untyped
// lookalike send remains opaque and therefore cannot gain a sharing proof.
func typedChannelReceiver(receiver []byte, partition equation.Partition) bool {
	if len(receiver) == 0 {
		return false
	}
	_, ok := typedChannelPayload(receiver, partition)
	return ok
}

// placementFactsFromChild transports closed allocation conclusions, never the
// child's private bindings.
func placementFactsFromChild(facts []equation.Fact) []equation.Fact {
	projected := make([]equation.Fact, 0)
	allocations := make(map[string]bool)
	projectedResults := make(map[string]string)
	resultOwners := make(map[string]string)
	ambiguousResults := make(map[string]bool)
	for _, fact := range facts {
		if allocation, ok := decodePlacementAllocation(fact); ok && allocation.Result != "" && allocation.Kind != "" {
			allocations[allocation.Identity] = true
			// Child terms are private to their lexical body.  Give every
			// published result a sealed identity-derived spelling so a caller's
			// coincidentally named temp/path cannot acquire child ownership.
			projectedResults[allocation.Identity] = "placement/projected/" + base64.RawURLEncoding.EncodeToString([]byte(allocation.Identity))
			if owner, found := resultOwners[allocation.Result]; !found && !ambiguousResults[allocation.Result] {
				resultOwners[allocation.Result] = allocation.Identity
			} else if owner != allocation.Identity {
				// A result spelling is not a unique child-graph edge.  Leave it
				// unrebound rather than choose one allocation by iteration order.
				delete(resultOwners, allocation.Result)
				ambiguousResults[allocation.Result] = true
			}
		}
	}
	for _, fact := range facts {
		switch {
		case strings.HasPrefix(fact.Key, placementAllocationPrefix):
			allocation, ok := decodePlacementAllocation(fact)
			if !ok || allocation.Result == "" || allocation.Kind == "" {
				continue
			}
			allocation.Result = projectedResults[allocation.Identity]
			for index, childResult := range allocation.Children {
				if childIdentity, found := resultOwners[childResult]; found {
					allocation.Children[index] = projectedResults[childIdentity]
				}
			}
			value, err := encodePlacementAllocation(allocation)
			if err != nil {
				continue
			}
			projected = append(projected, equation.Fact{Key: placementAllocationFactKey(allocation.Identity), Value: value})
		case strings.HasPrefix(fact.Key, placementEventPrefix), strings.HasPrefix(fact.Key, placementBlockerPrefix), strings.HasPrefix(fact.Key, placementContractPrefix):
			parts, ok := placementProvenFactParts(fact)
			if !ok {
				continue
			}
			identity, ok := placementFactIdentity(parts)
			if !ok || !allocations[identity] {
				continue
			}
			projected = append(projected, equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)})
		case strings.HasPrefix(fact.Key, placementContainmentPrefix):
			parts, ok := placementProvenFactParts(fact)
			if !ok {
				continue
			}
			parent, child, ok := placementContainmentIdentities(parts)
			if !ok || !allocations[parent] || !allocations[child] {
				continue
			}
			projected = append(projected, equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)})
		}
	}
	return projected
}

// placementStackWitnessFacts projects only complete child allocations with no
// observed ownership boundary. The child evaluator has already established
// the allocation identity and its entire local graph; retaining, sharing,
// containment, contracts, and blockers all disqualify the fact because those
// conclusions require a caller-owned invocation boundary.
func placementStackWitnessFacts(facts []equation.Fact) []equation.Fact {
	allocations := make(map[string]equation.Fact)
	boundary := make(map[string]bool)
	suspended := make(map[string][]equation.Fact)
	for _, fact := range facts {
		switch {
		case strings.HasPrefix(fact.Key, placementAllocationPrefix):
			if allocation, ok := decodePlacementAllocation(fact); ok && allocation.Complete {
				allocations[allocation.Identity] = cloneFact(fact)
			}
		case strings.HasPrefix(fact.Key, placementEventPrefix), strings.HasPrefix(fact.Key, placementBlockerPrefix), strings.HasPrefix(fact.Key, placementContractPrefix):
			parts, ok := placementProvenFactParts(fact)
			if !ok {
				continue
			}
			if identity, ok := placementFactIdentity(parts); ok {
				if strings.HasPrefix(fact.Key, placementEventPrefix) && parts[3] == placementEventSuspended {
					suspended[identity] = append(suspended[identity], cloneFact(fact))
				} else {
					boundary[identity] = true
				}
			}
		case strings.HasPrefix(fact.Key, placementContainmentPrefix):
			parts, ok := placementProvenFactParts(fact)
			if !ok {
				continue
			}
			if parent, child, ok := placementContainmentIdentities(parts); ok {
				boundary[parent], boundary[child] = true, true
			}
		}
	}
	identities := make([]string, 0, len(allocations))
	for identity := range allocations {
		if !boundary[identity] {
			identities = append(identities, identity)
		}
	}
	sort.Strings(identities)
	witnesses := make([]equation.Fact, 0, len(identities))
	for _, identity := range identities {
		witnesses = append(witnesses, allocations[identity])
		witnesses = append(witnesses, suspended[identity]...)
	}
	return placementFactsFromChild(witnesses)
}

type publishedPlacementFacts struct {
	allocations       map[string]placementAllocationFact
	bindings          map[string]string
	events            map[string]map[string]bool
	blockers          map[string]map[string]bool
	blockerOperations map[string]map[string]map[string]bool
	contracts         map[string]map[string]bool
	containment       map[string][]string
}

func parsePublishedPlacement(facts []equation.Fact) publishedPlacementFacts {
	parsed := publishedPlacementFacts{
		allocations: make(map[string]placementAllocationFact), bindings: make(map[string]string),
		events: make(map[string]map[string]bool), blockers: make(map[string]map[string]bool),
		blockerOperations: make(map[string]map[string]map[string]bool), contracts: make(map[string]map[string]bool), containment: make(map[string][]string),
	}
	for _, fact := range facts {
		parsePublishedPlacementFact(&parsed, fact)
	}
	return parsed
}

func parsePublishedPlacementFact(parsed *publishedPlacementFacts, fact equation.Fact) {
	switch {
	case strings.HasPrefix(fact.Key, placementAllocationPrefix):
		if allocation, ok := decodePlacementAllocation(fact); ok && allocation.Result != "" && allocation.Kind != "" {
			parsed.allocations[allocation.Identity] = allocation
		}
	case strings.HasPrefix(fact.Key, placementBindingPrefix):
		parts := strings.Split(fact.Key, "/")
		if len(parts) == 4 && len(fact.Value) != 0 {
			if term, err := base64.RawURLEncoding.DecodeString(parts[2]); err == nil {
				parsed.bindings[string(term)] = string(fact.Value)
			}
		}
	case strings.HasPrefix(fact.Key, placementEventPrefix), strings.HasPrefix(fact.Key, placementBlockerPrefix), strings.HasPrefix(fact.Key, placementContractPrefix):
		parts, ok := placementProvenFactParts(fact)
		if !ok {
			return
		}
		identity, ok := placementFactIdentity(parts)
		if !ok {
			return
		}
		if strings.HasPrefix(fact.Key, placementEventPrefix) {
			placementFactSet(parsed.events, identity)[parts[3]] = true
		} else if strings.HasPrefix(fact.Key, placementBlockerPrefix) {
			placementFactSet(parsed.blockers, identity)[parts[3]] = true
			if parsed.blockerOperations[identity] == nil {
				parsed.blockerOperations[identity] = make(map[string]map[string]bool)
			}
			if parsed.blockerOperations[identity][parts[3]] == nil {
				parsed.blockerOperations[identity][parts[3]] = make(map[string]bool)
			}
			parsed.blockerOperations[identity][parts[3]][parts[4]] = true
		} else {
			placementFactSet(parsed.contracts, identity)[parts[4]] = true
		}
	case strings.HasPrefix(fact.Key, placementContainmentPrefix):
		if parts, ok := placementProvenFactParts(fact); ok {
			if parent, child, ok := placementContainmentIdentities(parts); ok && parent != child {
				parsed.containment[parent] = append(parsed.containment[parent], child)
			}
		}
	}
}

func placementFactSet(facts map[string]map[string]bool, identity string) map[string]bool {
	if facts[identity] == nil {
		facts[identity] = make(map[string]bool)
	}
	return facts[identity]
}

func publishedPlacement(facts []equation.Fact) *PlacementPlan {
	parsed := parsePublishedPlacement(facts)
	if len(parsed.allocations) == 0 {
		return nil
	}
	return projectPublishedPlacement(parsed, propagatePublishedPlacement(parsed))
}

func propagatePublishedPlacement(parsed publishedPlacementFacts) map[string][]string {
	allocations, events, containment := parsed.allocations, parsed.events, parsed.containment
	children := make(map[string][]string, len(allocations))
	for identity, allocation := range allocations {
		for _, childTerm := range allocation.Children {
			if child, found := allocationsByResult(allocations, childTerm); found {
				children[identity] = append(children[identity], child)
			}
		}
		children[identity] = append(children[identity], containment[identity]...)
	}
	propagate := func(event string) {
		changed := true
		for changed {
			changed = false
			for parent, descendants := range children {
				if !events[parent][event] {
					continue
				}
				for _, child := range descendants {
					if events[child] == nil {
						events[child] = make(map[string]bool)
					}
					if !events[child][event] {
						events[child][event], changed = true, true
					}
				}
			}
		}
	}
	propagate(placementEventOwned)
	propagate(placementEventShared)
	propagate(placementEventSealed)
	propagate(placementEventSuspended)
	return children
}

func allocationsByResult(allocations map[string]placementAllocationFact, result string) (string, bool) {
	for identity, allocation := range allocations {
		if allocation.Result == result {
			return identity, true
		}
	}
	return "", false
}

func projectPublishedPlacement(parsed publishedPlacementFacts, children map[string][]string) *PlacementPlan {
	allocations, bindings, events := parsed.allocations, parsed.bindings, parsed.events
	blockers, blockerOperations, contracts := parsed.blockers, parsed.blockerOperations, parsed.contracts
	allocationDepth := placementAllocationDepth(allocations, children)
	plan := &PlacementPlan{Complete: true, Allocations: make([]PlacementAllocation, 0, len(allocations))}
	for identity, allocation := range allocations {
		item := projectPlacementAllocation(identity, allocation, allocationDepth(identity), bindings, events, blockers, blockerOperations, contracts)
		if !item.Complete {
			plan.Complete = false
		}
		plan.Allocations = append(plan.Allocations, item)
	}
	sort.Slice(plan.Allocations, func(i, j int) bool { return plan.Allocations[i].Identity < plan.Allocations[j].Identity })
	return plan
}

func placementAllocationDepth(allocations map[string]placementAllocationFact, children map[string][]string) func(string) int {
	depth := make(map[string]int, len(allocations))
	var allocationDepth func(string, map[string]bool) int
	allocationDepth = func(identity string, visiting map[string]bool) int {
		if value := depth[identity]; value != 0 {
			return value
		}
		if visiting[identity] {
			return 1
		}
		visiting[identity] = true
		value := 1
		for _, child := range children[identity] {
			if candidate := 1 + allocationDepth(child, visiting); candidate > value {
				value = candidate
			}
		}
		delete(visiting, identity)
		depth[identity] = value
		return value
	}
	return func(identity string) int { return allocationDepth(identity, make(map[string]bool)) }
}

func projectPlacementAllocation(identity string, allocation placementAllocationFact, depth int, bindings map[string]string, events, blockers map[string]map[string]bool, blockerOperations map[string]map[string]map[string]bool, contracts map[string]map[string]bool) PlacementAllocation {
	item := PlacementAllocation{Identity: identity, Target: allocation.Result, Kind: allocation.Kind, Complete: allocation.Complete, Depth: depth}
	for term, bound := range bindings {
		if bound == identity {
			item.Target = term
			break
		}
	}
	for blocker := range blockers[identity] {
		if blocker == "opaque-call" {
			allContracted := len(blockerOperations[identity][blocker]) != 0
			for operation := range blockerOperations[identity][blocker] {
				allContracted = allContracted && contracts[identity][operation]
			}
			if allContracted {
				continue
			}
		}
		item.Blockers = append(item.Blockers, blocker)
	}
	sort.Strings(item.Blockers)
	switch {
	case !item.Complete || len(item.Blockers) != 0:
		item.Placement = placement.Unknown
	case events[identity][placementEventShared]:
		item.Placement, item.SealBeforeShare = placement.SharedHeap, events[identity][placementEventSealed]
		if !item.SealBeforeShare {
			item.Obligations = append(item.Obligations, "seal-before-share")
		}
	case events[identity][placementEventOwned]:
		item.Placement, item.OwnerIdentity = placement.OwnedHeap, true
	default:
		item.Placement, item.HasDiesBeforeSuspension = placement.Stack, true
		if events[identity][placementEventSuspended] {
			// A published suspend edge refutes the two lifetime licenses without
			// changing the allocation's stack/heap ownership classification.
			item.FrameLocal, item.DiesBeforeSuspension = false, false
		} else {
			item.FrameLocal, item.DiesBeforeSuspension = true, true
		}
	}
	item.Decomposable = allocation.Decomposable && item.Placement == placement.Stack && len(item.Blockers) == 0 && len(contracts[identity]) == 0
	return item
}
