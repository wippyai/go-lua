package engine

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/placement"
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
	placementAllocationPrefix = "placement/allocation/"
	placementBindingPrefix    = "placement/binding/"
	placementEventPrefix      = "placement/event/"
	placementBlockerPrefix    = "placement/blocker/"

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
		case operands.display == "ownership.store" && index == 0:
			facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventOwned))
		case operands.display == "process.send" && index == 2:
			facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventShared))
		case operands.display == "table.freeze" && index == 0:
			facts = append(facts, placementEventFact(allocation.Identity, operation.Target.Name, placementEventSealed))
		default:
			facts = append(facts, placementBlockerFact(allocation.Identity, operation.Target.Name, "opaque-call"))
		}
	}
	return facts
}

func publishedPlacement(facts []equation.Fact) *PlacementPlan {
	allocations := make(map[string]placementAllocationFact)
	bindings := make(map[string]string)
	events := make(map[string]map[string]bool)
	blockers := make(map[string]map[string]bool)
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
