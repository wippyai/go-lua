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

func TestInactiveRelationCatalogFailsClosedForRecursiveAndContextualShapes(t *testing.T) {
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
	config := Config{Check: check}
	config.relationCatalogAudit = func(catalog relationRunCatalog) error {
		entries := catalog.Entries()
		if len(entries) != 1 || entries[0].Summary != exactKey {
			return fmt.Errorf("eligible identities = %#v, want only exact %v", entries, exactKey)
		}
		return nil
	}
	if _, err := RunBoundChunk(stmts, bindings, config); err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
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
