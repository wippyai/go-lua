package program

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestInactiveRelationCatalogUsesDeterministicPairedIdentityAndExactRoutes(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(x: any): any
    return x
end
local function caller(x: any): any
    return leaf(x)
end
return caller("ok")
`)
	reg := standard.Registry()
	check := body.Config{Registry: reg}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, check.ModuleTypes, check.ModuleExports, stmts)
	leafKey := relationTestTargetKey(t, bindings, keys, "leaf")
	callerKey := relationTestTargetKey(t, bindings, keys, "caller")

	type snapshotEntry struct {
		Cell   transformer.CellRef
		Key    summary.SummaryKey
		Digest uint64
		Routes []transformer.CellRef
	}
	var snapshots [][]snapshotEntry
	for run := 0; run < 2; run++ {
		config := Config{Check: check}
		config.relationCatalogAudit = func(catalog relationRunCatalog) error {
			entries := catalog.Entries()
			got := make([]snapshotEntry, 0, len(entries))
			for _, identity := range entries {
				direct, ok := catalog.DirectCalls(identity)
				if !ok {
					return fmt.Errorf("complete identity rejected: %#v", identity)
				}
				var routes []transformer.CellRef
				for point := cfg.Point(0); int(point) < direct.PointCount(); point++ {
					if target, exists := direct.Lookup(point); exists {
						routes = append(routes, target.Cell)
					}
				}
				got = append(got, snapshotEntry{Cell: identity.Cell, Key: identity.Summary, Digest: identity.BodyDigest, Routes: routes})
			}
			if len(entries) != 2 || entries[0].Summary != leafKey || entries[1].Summary != callerKey {
				return fmt.Errorf("catalog keys = %v, want leaf then caller in SummaryKey order", got)
			}
			leafCell := entries[0].Cell
			callerDirect, _ := catalog.DirectCalls(entries[1])
			matched := 0
			for point := cfg.Point(0); int(point) < callerDirect.PointCount(); point++ {
				if target, ok := callerDirect.Lookup(point); ok {
					matched++
					if target.Cell != leafCell || target.Shape.Params != 1 {
						return fmt.Errorf("caller target = %#v, want leaf %v with one parameter", target, leafCell)
					}
				}
			}
			if matched != 1 {
				return fmt.Errorf("caller routes = %d, want exactly one", matched)
			}

			// Each field is part of authority. No independently matching key,
			// digest, cell, or prepared body can read the route catalog.
			for name, mutate := range map[string]func(*relationCellIdentity){
				"cell":     func(id *relationCellIdentity) { id.Cell.Function++ },
				"summary":  func(id *relationCellIdentity) { id.Summary.Entry.Values++ },
				"digest":   func(id *relationCellIdentity) { id.BodyDigest++ },
				"prepared": func(id *relationCellIdentity) { id.Prepared = nil },
			} {
				mismatch := entries[1]
				mutate(&mismatch)
				if _, ok := catalog.DirectCalls(mismatch); ok {
					return fmt.Errorf("%s identity mismatch was accepted", name)
				}
			}
			snapshots = append(snapshots, got)
			return nil
		}
		if _, err := RunBoundChunk(stmts, bindings, config); err != nil {
			t.Fatalf("run %d: RunBoundChunk: %v", run, err)
		}
	}
	if !reflect.DeepEqual(snapshots[0], snapshots[1]) {
		t.Fatalf("catalog is not deterministic\nfirst:  %#v\nsecond: %#v", snapshots[0], snapshots[1])
	}
}

func TestInactiveRelationCatalogFailsClosedForRecursiveAndUnsupportedShapes(t *testing.T) {
	stmts := parseChunk(t, `
local function exact(x: any): any
    return x
end
local function self(x: any): any
    return self(x)
end
local left: any
local right: any
left = function(x: any): any return right(x) end
right = function(x: any): any return left(x) end
local captured = "captured"
local function capture(x: any): any
    return captured
end
local function variadic(...: any): any
    return ...
end
local function mutates(x: any): any
    local value = x
    value = "changed"
    return value
end
local function allocates(): any
    return table.create(1)
end
local object = {}
function object:method(x: any): any
    return x
end
return exact("ok")
`)
	reg := standard.Registry()
	check := body.Config{Registry: reg}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, check.ModuleTypes, check.ModuleExports, stmts)
	exactKey := relationTestTargetKey(t, bindings, keys, "exact")
	captureKey := relationTestTargetKey(t, bindings, keys, "capture")
	config := Config{Check: check}
	config.relationCatalogAudit = func(catalog relationRunCatalog) error {
		entries := catalog.Entries()
		if len(entries) != 2 || entries[0].Summary != exactKey || entries[1].Summary != captureKey {
			return fmt.Errorf("eligible identities = %#v, want exact %v plus certificate-gated capture %v", entries, exactKey, captureKey)
		}
		return nil
	}
	if _, err := RunBoundChunk(stmts, bindings, config); err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
}

func TestInactiveRelationConsumerPolicySeparatesCallersFromProducers(t *testing.T) {
	stmts := parseChunk(t, `
local captured = "captured"
local function leaf(x: any): any
    return x
end
local function captured_caller(x: any): any
    if captured then
        return leaf(x)
    end
    return x
end
local function unrelated(x: any): any
    return x
end
return leaf(captured_caller("ok"))
`)
	reg := standard.Registry()
	check := body.Config{Registry: reg}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, check.ModuleTypes, check.ModuleExports, stmts)
	leafKey := relationTestTargetKey(t, bindings, keys, "leaf")
	callerKey := relationTestTargetKey(t, bindings, keys, "captured_caller")
	unrelatedKey := relationTestTargetKey(t, bindings, keys, "unrelated")

	type consumerSnapshot struct {
		Key    summary.SummaryKey
		Digest uint64
		Active bool
		Routes []transformer.CellRef
	}
	var snapshots [][]consumerSnapshot
	for run := 0; run < 2; run++ {
		config := Config{Check: check}
		config.relationCatalogAudit = func(catalog relationRunCatalog) error {
			policy := catalog.ConsumerPolicy()
			owners := policy.Owners()
			byKey := make(map[summary.SummaryKey]relationConsumerIdentity, len(owners))
			got := make([]consumerSnapshot, 0, len(owners))
			producerCells := make(map[summary.SummaryKey]transformer.CellRef)
			for _, producer := range catalog.Entries() {
				producerCells[producer.Summary] = producer.Cell
			}
			leafCell, leafAdmitted := producerCells[leafKey]
			if !leafAdmitted {
				return fmt.Errorf("leaf was not admitted as a producer: %#v", catalog.Entries())
			}
			if _, admitted := producerCells[callerKey]; admitted {
				return fmt.Errorf("capture-bearing caller composed before capture-root call-flow proof")
			}
			for _, owner := range owners {
				byKey[owner.Summary] = owner
				direct, ok := policy.DirectCalls(owner)
				if !ok {
					return fmt.Errorf("complete consumer identity rejected: %#v", owner)
				}
				var routes []transformer.CellRef
				for point := cfg.Point(0); int(point) < direct.PointCount(); point++ {
					if target, exists := direct.Lookup(point); exists {
						routes = append(routes, target.Cell)
						if target.Cell != leafCell {
							return fmt.Errorf("consumer %v routed to non-leaf target %v; admitted=%v", owner.Summary, target.Cell, producerCells)
						}
					}
				}
				got = append(got, consumerSnapshot{Key: owner.Summary, Digest: owner.BodyDigest, Active: policy.Active(owner), Routes: routes})
			}
			for key, wantActive := range map[summary.SummaryKey]bool{
				keys.rootKey: true, callerKey: true, leafKey: false, unrelatedKey: false,
			} {
				owner, ok := byKey[key]
				if !ok {
					return fmt.Errorf("consumer owner %v missing", key)
				}
				if active := policy.Active(owner); active != wantActive {
					return fmt.Errorf("consumer %v active = %v, want %v", key, active, wantActive)
				}
			}
			if len(got) != 4 {
				return fmt.Errorf("consumer owners = %d, want chunk root plus three lexical functions", len(got))
			}
			if len(got[0].Routes)+len(got[1].Routes)+len(got[2].Routes)+len(got[3].Routes) != 2 {
				return fmt.Errorf("consumer routes = %#v, want root and captured caller only", got)
			}

			owner := byKey[callerKey]
			for name, mutate := range map[string]func(*relationConsumerIdentity){
				"summary":  func(id *relationConsumerIdentity) { id.Summary.Entry.Values++ },
				"digest":   func(id *relationConsumerIdentity) { id.BodyDigest++ },
				"prepared": func(id *relationConsumerIdentity) { id.Prepared = nil },
			} {
				mismatch := owner
				mutate(&mismatch)
				if policy.Active(mismatch) {
					return fmt.Errorf("%s identity mismatch was active", name)
				}
				if _, ok := policy.DirectCalls(mismatch); ok {
					return fmt.Errorf("%s identity mismatch read routes", name)
				}
			}
			snapshots = append(snapshots, got)
			return nil
		}
		if _, err := RunBoundChunk(stmts, bindings, config); err != nil {
			t.Fatalf("run %d: RunBoundChunk: %v", run, err)
		}
	}
	if !reflect.DeepEqual(snapshots[0], snapshots[1]) {
		t.Fatalf("consumer policy is not deterministic\nfirst:  %#v\nsecond: %#v", snapshots[0], snapshots[1])
	}
}

func TestInactiveRelationConsumerPolicyIncludesRunBoundFunctionRoot(t *testing.T) {
	stmts := parseChunk(t, `
local function root(x: any): any
    local function leaf(y: any): any
        return y
    end
    return leaf(x)
end
`)
	reg := standard.Registry()
	check := body.Config{Registry: reg}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	var rootFn *ast.FunctionExpr
	bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.HasTargetSymbol && bindings.Name(origin.TargetSymbol) == "root" {
			rootFn = origin.Func
			return false
		}
		return true
	})
	if rootFn == nil {
		t.Fatal("root function not found")
	}
	config := Config{Check: check}
	config.relationCatalogAudit = func(catalog relationRunCatalog) error {
		policy := catalog.ConsumerPolicy()
		for _, owner := range policy.Owners() {
			if owner.Summary == rootKey(config.RootKey) {
				if !policy.Active(owner) {
					return fmt.Errorf("RunBoundFunction root is not an active relation consumer")
				}
				return nil
			}
		}
		return fmt.Errorf("RunBoundFunction root consumer missing")
	}
	if _, err := RunBoundFunction(rootFn, bindings, config); err != nil {
		t.Fatalf("RunBoundFunction: %v", err)
	}
}

func relationTestTargetKey(t testing.TB, bindings *bind.Result, keys programKeys, name string) summary.SummaryKey {
	t.Helper()
	var found summary.SummaryKey
	bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.HasTargetSymbol && bindings.Name(origin.TargetSymbol) == name {
			found = keys.targetKeys[origin.TargetSymbol]
			return false
		}
		return true
	})
	if found.Ref.IsZero() {
		t.Fatalf("target key %q not found", name)
	}
	return found
}
