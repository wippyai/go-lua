package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestTargetSelectorCandidatesPreserveDuplicateNonmonotonicOrdinals proves
// that candidate ordinal is a vector position, not target declaration order.
func TestTargetSelectorCandidatesPreserveDuplicateNonmonotonicOrdinals(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var units [3]carrier.Unit
	var targets [3]carrier.Target
	var selector carrier.Selector
	binding, ok := bindTest(capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		var declared bool
		for index := range units {
			units[index], declared = binding.DeclareExact(uint64(index))
			if !declared {
				return false
			}
		}
		for index := range targets {
			targets[index], declared = binding.DeclareStrong(units[index])
			if !declared {
				return false
			}
		}
		selector, declared = binding.DeclareTargetSelector([]carrier.Target{targets[2], targets[0], targets[2], targets[1]})
		return declared
	}), manager)
	if !ok {
		t.Fatal("binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare")
	}
	selected, ok := prepared.SelectorTargets(shape.Slot(0), selector)
	if !ok {
		t.Fatal("target selector")
	}
	want := []carrier.Target{targets[2], targets[0], targets[2], targets[1]}
	if !sameTargetVector(selected, want) {
		t.Fatal("target selector reordered or deduplicated positional candidates")
	}
}

// TestTargetSelectorCandidatesRejectForeignBindingCapabilities ensures every
// target comes from the exact Binding declaration epoch.
func TestTargetSelectorCandidatesRejectForeignBindingCapabilities(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var foreignTarget carrier.Target
	if _, ok := bindTest(capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		unit, declared := binding.DeclareExact(0)
		if !declared {
			return false
		}
		foreignTarget, declared = binding.DeclareStrong(unit)
		return declared
	}), manager); !ok {
		t.Fatal("foreign binding")
	}
	if _, ok := bindTest(capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		unit, declared := binding.DeclareExact(0)
		if !declared {
			return false
		}
		target, declared := binding.DeclareStrong(unit)
		if !declared {
			return false
		}
		_, declared = binding.DeclareTargetSelector([]carrier.Target{target, foreignTarget})
		return !declared
	}), manager); !ok {
		t.Fatal("foreign selector candidates were accepted")
	}
}

func sameTargetVector(left, right []carrier.Target) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].Same(right[index]) {
			return false
		}
	}
	return true
}
