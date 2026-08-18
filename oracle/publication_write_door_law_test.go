// publication_write_door_law_test.go states the one-write-door law over the
// whole module: a published column changes through the write capability the
// engine mints for it, and through nothing else.
//
// The law is stated here rather than beside either end of it. It is a statement
// about the module as a whole - that no package anywhere reaches storage on its
// own - so it must be read from the module's own sources, and this package
// already reads them black-box for the architecture battery.

package oracle

import (
	"go/ast"
	"strconv"
	"testing"
)

// publicationWriteVerbs are the snapshot package's storage write verbs: the
// four calls that decide what a published column holds. Every one of them takes
// a caller-supplied address, so a caller that reaches one directly writes a
// column no principal was admitted to write.
var publicationWriteVerbs = map[string]struct{}{
	"PutColumn":    {},
	"SetRow":       {},
	"RemoveRow":    {},
	"DeclareQuery": {},
}

const (
	// publicationWriteDoorOwner is the one production package that reaches the
	// write verbs. It is the package that owns the capability: it mints one
	// token per admitted column and wraps each verb behind it, so the address a
	// write lands at travels with the token rather than with the caller.
	publicationWriteDoorOwner = "analysis/engine"
	// publicationWriteDoorStorage is the package that declares the verbs. It is
	// the storage itself and knows nothing of principals.
	publicationWriteDoorStorage = "analysis/snapshot"
	// publicationSnapshotImport is the import path the verbs are named through.
	publicationSnapshotImport = "github.com/wippyai/go-lua/analysis/snapshot"
)

// TestOneWriteDoorOverEveryPublishedColumn is the closed law. No production
// source outside the capability owner and the storage itself names a snapshot
// write verb, so every column of every publication is filled through a minted
// ColumnWrite.
//
// Tests are exempt on purpose. A law about a column's read contract fills one
// directly to state what a reader concludes from it; what this law protects is
// the analyzer a consumer receives, and a test binary is not part of it.
func TestOneWriteDoorOverEveryPublishedColumn(t *testing.T) {
	found := make([]string, 0)
	architectureBatteryWalk(t, ".", func(source architectureBatterySource) {
		if source.test {
			return
		}
		if directory := source.directory(); directory == publicationWriteDoorOwner || directory == publicationWriteDoorStorage {
			return
		}
		qualifier, imported := publicationSnapshotQualifier(t, source)
		if !imported {
			return
		}
		for _, verb := range publicationWriteVerbCalls(source, qualifier) {
			found = append(found, source.path+": "+qualifier+"."+verb)
		}
	})
	for _, call := range found {
		t.Errorf("a published column is written outside the engine's write capability: %s", call)
	}
}

// publicationWriteDoorInterior freezes the capability owner's own uses of the
// write verbs. The wrapper file is the door itself and holds one use per verb;
// every other entry is a publication path that still addresses storage on its
// own, and the count is how many verb calls that path makes.
//
// It is a ratchet rather than a closed law because the interior is where the
// remaining paths retire. An entry disappears when its path does, so the
// inventory measures the retirement instead of asserting it is finished.
var publicationWriteDoorInterior = map[string]int{
	"analysis/engine/publication_column.go": 4,
}

// TestTheCapabilityOwnersInteriorWriteSitesOnlyShrink holds the one exempt
// package to its own inventory. A new production file inside the engine that
// reaches storage directly fails here, and a retiring path that goes away is
// reported so the inventory can be trimmed.
func TestTheCapabilityOwnersInteriorWriteSitesOnlyShrink(t *testing.T) {
	found := make(map[string]int)
	architectureBatteryWalk(t, publicationWriteDoorOwner, func(source architectureBatterySource) {
		if source.test || source.directory() != publicationWriteDoorOwner {
			return
		}
		qualifier, imported := publicationSnapshotQualifier(t, source)
		if !imported {
			return
		}
		if calls := publicationWriteVerbCalls(source, qualifier); len(calls) > 0 {
			found[source.path] = len(calls)
		}
	})
	architectureBatteryRatchet(t, "the engine's interior write sites only shrink", publicationWriteDoorInterior, found)
}

// TestAnalyzeDoesNotCallCompositeMaterialize is the W1 endpoint law. Analyze
// publishes through Snapshot Commit. The composite materializer is not a
// production publisher.
// TestSnapshotWriteVerbsAreReachedOnlyThroughColumnWrite is the capability
// half of the door. A snapshot write verb named inside the engine must sit in
// a function that takes a ColumnWrite: address-only wrappers are the bypass
// the name-matching ratchet cannot see.
func TestSnapshotWriteVerbsAreReachedOnlyThroughColumnWrite(t *testing.T) {
	architectureBatteryWalk(t, publicationWriteDoorOwner, func(source architectureBatterySource) {
		if source.test || source.directory() != publicationWriteDoorOwner {
			return
		}
		qualifier, imported := publicationSnapshotQualifier(t, source)
		if !imported {
			return
		}
		for _, decl := range source.file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Body == nil {
				continue
			}
			if !functionCallsPublicationWriteVerb(fn, qualifier) {
				continue
			}
			if !functionHasColumnWriteParam(fn) {
				t.Errorf("%s: %s reaches a snapshot write verb without a ColumnWrite parameter", source.path, fn.Name.Name)
			}
		}
	})
}

func functionCallsPublicationWriteVerb(fn *ast.FuncDecl, qualifier string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		callee := call.Fun
		switch indexed := callee.(type) {
		case *ast.IndexExpr:
			callee = indexed.X
		case *ast.IndexListExpr:
			callee = indexed.X
		}
		selector, isSelector := callee.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		base, isIdent := selector.X.(*ast.Ident)
		if !isIdent || base.Name != qualifier {
			return true
		}
		if _, isVerb := publicationWriteVerbs[selector.Sel.Name]; isVerb {
			found = true
		}
		return true
	})
	return found
}

func functionHasColumnWriteParam(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		expr := field.Type
		switch typed := expr.(type) {
		case *ast.IndexExpr:
			expr = typed.X
		case *ast.IndexListExpr:
			expr = typed.X
		}
		ident, isIdent := expr.(*ast.Ident)
		if isIdent && ident.Name == "ColumnWrite" {
			return true
		}
	}
	return false
}

func TestAnalyzeDoesNotCallQueryPublisher(t *testing.T) {
	architectureBatteryWalk(t, "analysis", func(source architectureBatterySource) {
		if source.test {
			return
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			switch callee := call.Fun.(type) {
			case *ast.Ident:
				if callee.Name == "NewQueryPublisher" || callee.Name == "AddQueryPublisher" {
					t.Errorf("%s calls %s", source.path, callee.Name)
				}
			case *ast.IndexExpr:
				if ident, isIdent := callee.X.(*ast.Ident); isIdent && (ident.Name == "NewQueryPublisher" || ident.Name == "AddQueryPublisher") {
					t.Errorf("%s calls %s", source.path, ident.Name)
				}
			case *ast.IndexListExpr:
				if ident, isIdent := callee.X.(*ast.Ident); isIdent && (ident.Name == "NewQueryPublisher" || ident.Name == "AddQueryPublisher") {
					t.Errorf("%s calls %s", source.path, ident.Name)
				}
			}
			return true
		})
	})
}

func TestAnalyzeDoesNotCallCompositeMaterialize(t *testing.T) {
	architectureBatteryWalk(t, "analysis", func(source architectureBatterySource) {
		if source.test || source.directory() != "analysis" {
			return
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "Materialize" {
				return true
			}
			base, isIdent := selector.X.(*ast.Ident)
			if isIdent && base.Name == "composite" {
				t.Errorf("%s calls composite.Materialize", source.path)
			}
			return true
		})
	})
}

// publicationSnapshotQualifier is the name one source calls the snapshot
// package by, and whether it imports it at all. A source that does not import
// it cannot name a write verb.
func publicationSnapshotQualifier(t *testing.T, source architectureBatterySource) (string, bool) {
	t.Helper()
	for _, imported := range source.file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("%s: unquote import %s: %v", source.path, imported.Path.Value, err)
		}
		if path != publicationSnapshotImport {
			continue
		}
		if imported.Name != nil {
			return imported.Name.Name, imported.Name.Name != "_"
		}
		return publicationWriteDoorStorage[len("analysis/"):], true
	}
	return "", false
}

// publicationWriteVerbCalls lists the write verbs one source calls through the
// qualifier it imported the storage under. Only a call is a write: naming a
// verb without calling it writes nothing.
func publicationWriteVerbCalls(source architectureBatterySource, qualifier string) []string {
	verbs := make([]string, 0)
	ast.Inspect(source.file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		// A generic verb is called through an index expression, so the callee
		// is unwrapped before it is read as a qualified name.
		callee := call.Fun
		switch indexed := callee.(type) {
		case *ast.IndexExpr:
			callee = indexed.X
		case *ast.IndexListExpr:
			callee = indexed.X
		}
		selector, isSelector := callee.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		base, isIdent := selector.X.(*ast.Ident)
		if !isIdent || base.Name != qualifier {
			return true
		}
		if _, isVerb := publicationWriteVerbs[selector.Sel.Name]; isVerb {
			verbs = append(verbs, selector.Sel.Name)
		}
		return true
	})
	return verbs
}
