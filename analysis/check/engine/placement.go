package engine

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/placement"
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

	placementEventOwned  = "owned"
	placementEventShared = "shared"
	placementEventSealed = "sealed"
)

type placementAllocationFact struct {
	Identity     string   `json:"identity"`
	Result       string   `json:"result"`
	Kind         string   `json:"kind"`
	Complete     bool     `json:"complete"`
	Decomposable bool     `json:"decomposable"`
	Children     []string `json:"children,omitempty"`
}

func encodePlacementAllocation(fact placementAllocationFact) ([]byte, error) {
	return json.Marshal(fact)
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
		return placementAllocationByIdentity(identity, partition)
	}
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, placementAllocationPrefix) {
			continue
		}
		var allocation placementAllocationFact
		if json.Unmarshal(fact.Value, &allocation) == nil && allocation.Identity != "" && allocation.Result == string(term) {
			return allocation, true
		}
	}
	return placementAllocationFact{}, false
}

func placementAllocationByIdentity(identity string, partition equation.Partition) (placementAllocationFact, bool) {
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, placementAllocationPrefix) {
			continue
		}
		var allocation placementAllocationFact
		if json.Unmarshal(fact.Value, &allocation) == nil && allocation.Identity == identity {
			return allocation, true
		}
	}
	return placementAllocationFact{}, false
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
			continue
		}
		switch {
		case operands.display == "ownership.store" && (index == 0 || index == 1):
			// The ownership contract proves both the stored graph and its owner
			// are retained past the caller frame. No opaque fallback is needed
			// for either exact contract argument.
			facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventOwned))
		case operands.display == "process.send" && index == 2:
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
		default:
			facts = append(facts, placementBlockerFact(allocation.Identity, operation.Target.Name, "opaque-call"))
		}
	}
	return facts
}

// placementFactsFromChild transports only already-closed allocation conclusions
// from a lexical evaluation. Bindings name the child's private frame and are
// deliberately not projected; allocation identities and their boundary facts
// are self-contained and remain valid at the caller publication boundary.
func placementFactsFromChild(facts []equation.Fact) []equation.Fact {
	projected := make([]equation.Fact, 0)
	allocations := make(map[string]bool)
	projectedResults := make(map[string]string)
	resultOwners := make(map[string]string)
	ambiguousResults := make(map[string]bool)
	for _, fact := range facts {
		if !strings.HasPrefix(fact.Key, placementAllocationPrefix) {
			continue
		}
		var allocation placementAllocationFact
		if json.Unmarshal(fact.Value, &allocation) == nil && allocation.Identity != "" && allocation.Result != "" && allocation.Kind != "" {
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
			var allocation placementAllocationFact
			if json.Unmarshal(fact.Value, &allocation) != nil || allocation.Identity == "" || allocation.Result == "" || allocation.Kind == "" {
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
			parts := strings.Split(fact.Key, "/")
			if len(parts) != 5 || string(fact.Value) != "proven" {
				continue
			}
			identity, err := base64.RawURLEncoding.DecodeString(parts[2])
			if err != nil || !allocations[string(identity)] {
				continue
			}
			projected = append(projected, equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)})
		case strings.HasPrefix(fact.Key, placementContainmentPrefix):
			parts := strings.Split(fact.Key, "/")
			if len(parts) != 5 || string(fact.Value) != "proven" {
				continue
			}
			parent, parentErr := base64.RawURLEncoding.DecodeString(parts[2])
			if parentErr != nil || !allocations[string(parent)] {
				continue
			}
			child, childErr := base64.RawURLEncoding.DecodeString(parts[3])
			if childErr != nil || !allocations[string(child)] {
				continue
			}
			projected = append(projected, equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)})
		}
	}
	return projected
}

func publishedPlacement(facts []equation.Fact) *PlacementPlan {
	allocations := make(map[string]placementAllocationFact)
	bindings := make(map[string]string)
	events := make(map[string]map[string]bool)
	blockers := make(map[string]map[string]bool)
	blockerOperations := make(map[string]map[string]map[string]bool)
	contracts := make(map[string]map[string]bool)
	containment := make(map[string][]string)
	for _, fact := range facts {
		switch {
		case strings.HasPrefix(fact.Key, placementAllocationPrefix):
			var allocation placementAllocationFact
			if json.Unmarshal(fact.Value, &allocation) == nil && allocation.Identity != "" && allocation.Result != "" && allocation.Kind != "" {
				allocations[allocation.Identity] = allocation
			}
		case strings.HasPrefix(fact.Key, placementBindingPrefix):
			parts := strings.Split(fact.Key, "/")
			if len(parts) == 4 && len(fact.Value) != 0 {
				term, err := base64.RawURLEncoding.DecodeString(parts[2])
				if err == nil {
					bindings[string(term)] = string(fact.Value)
				}
			}
		case strings.HasPrefix(fact.Key, placementEventPrefix):
			parts := strings.Split(fact.Key, "/")
			if len(parts) == 5 && string(fact.Value) == "proven" {
				identity, err := base64.RawURLEncoding.DecodeString(parts[2])
				if err == nil {
					if events[string(identity)] == nil {
						events[string(identity)] = make(map[string]bool)
					}
					events[string(identity)][parts[3]] = true
				}
			}
		case strings.HasPrefix(fact.Key, placementBlockerPrefix):
			parts := strings.Split(fact.Key, "/")
			if len(parts) == 5 && string(fact.Value) == "proven" {
				identity, err := base64.RawURLEncoding.DecodeString(parts[2])
				if err == nil {
					if blockers[string(identity)] == nil {
						blockers[string(identity)] = make(map[string]bool)
					}
					blockers[string(identity)][parts[3]] = true
					if blockerOperations[string(identity)] == nil {
						blockerOperations[string(identity)] = make(map[string]map[string]bool)
					}
					if blockerOperations[string(identity)][parts[3]] == nil {
						blockerOperations[string(identity)][parts[3]] = make(map[string]bool)
					}
					blockerOperations[string(identity)][parts[3]][parts[4]] = true
				}
			}
		case strings.HasPrefix(fact.Key, placementContractPrefix):
			parts := strings.Split(fact.Key, "/")
			if len(parts) == 5 && (parts[3] == "send" || parts[3] == "retain") && string(fact.Value) == "proven" {
				identity, err := base64.RawURLEncoding.DecodeString(parts[2])
				if err == nil {
					if contracts[string(identity)] == nil {
						contracts[string(identity)] = make(map[string]bool)
					}
					contracts[string(identity)][parts[4]] = true
				}
			}
		case strings.HasPrefix(fact.Key, placementContainmentPrefix):
			parts := strings.Split(fact.Key, "/")
			if len(parts) == 5 && string(fact.Value) == "proven" {
				parent, parentErr := base64.RawURLEncoding.DecodeString(parts[2])
				child, childErr := base64.RawURLEncoding.DecodeString(parts[3])
				if parentErr == nil && childErr == nil && string(parent) != string(child) {
					containment[string(parent)] = append(containment[string(parent)], string(child))
				}
			}
		}
	}
	if len(allocations) == 0 {
		return nil
	}
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
	plan := &PlacementPlan{Complete: true, Allocations: make([]PlacementAllocation, 0, len(allocations))}
	for identity, allocation := range allocations {
		item := PlacementAllocation{Identity: identity, Target: allocation.Result, Kind: allocation.Kind, Complete: allocation.Complete, Depth: allocationDepth(identity, make(map[string]bool))}
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
			item.Placement, item.FrameLocal, item.DiesBeforeSuspension, item.HasDiesBeforeSuspension = placement.Stack, true, true, true
		}
		item.Decomposable = allocation.Decomposable && item.Placement == placement.Stack && len(item.Blockers) == 0
		if !item.Complete {
			plan.Complete = false
		}
		plan.Allocations = append(plan.Allocations, item)
	}
	sort.Slice(plan.Allocations, func(i, j int) bool { return plan.Allocations[i].Identity < plan.Allocations[j].Identity })
	return plan
}

func allocationsByResult(allocations map[string]placementAllocationFact, result string) (string, bool) {
	for identity, allocation := range allocations {
		if allocation.Result == result {
			return identity, true
		}
	}
	return "", false
}
