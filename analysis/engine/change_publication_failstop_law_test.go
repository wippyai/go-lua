package engine

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// A publication path installs a new PointState into epoch.points before the
// last fallible step that follows it. Every false return after that install
// therefore leaves a half-landed commit, and the only thing that keeps it
// unobservable is the epoch fail-stop: canceled() gates every point visit,
// region visit and carrier operation in the executor.
//
// The law is structural because the hazard is structural. It selects its
// subjects by shape -- an executorEpoch method returning two booleans whose
// body installs into epoch.points -- so a new publication path is covered the
// moment it is written.
func TestNoPublicationPathReturnsFalseAfterInstallWithoutFailStop(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	subjects := 0
	for _, parsed := range packages {
		for _, file := range parsed.Files {
			for _, declaration := range file.Decls {
				function, isFunction := declaration.(*ast.FuncDecl)
				if !isFunction || !publicationPathShape(function) {
					continue
				}
				install := pointInstallPosition(function)
				if install == token.NoPos {
					continue
				}
				subjects++
				auditPublicationReturns(t, fset, function, install)
			}
		}
	}
	if subjects == 0 {
		t.Fatal("no publication path found: the law selects nothing and would pass vacuously")
	}
}

// publicationPathShape selects an executorEpoch method returning two booleans.
func publicationPathShape(function *ast.FuncDecl) bool {
	if function.Body == nil || function.Recv == nil || len(function.Recv.List) != 1 {
		return false
	}
	pointer, isPointer := function.Recv.List[0].Type.(*ast.StarExpr)
	if !isPointer {
		return false
	}
	name, isName := pointer.X.(*ast.Ident)
	if !isName || name.Name != "executorEpoch" {
		return false
	}
	results := function.Type.Results
	if results == nil {
		return false
	}
	count := 0
	for _, result := range results.List {
		identifier, isIdentifier := result.Type.(*ast.Ident)
		if !isIdentifier || identifier.Name != "bool" {
			return false
		}
		count += max(len(result.Names), 1)
	}
	return count == 2
}

// pointInstallPosition returns the first install into the published point
// plane. The install itself lives behind epoch.installPoint, so a publication
// path is recognized by that call; a direct assignment is still recognized so
// the law survives a path that reintroduces one.
func pointInstallPosition(function *ast.FuncDecl) token.Pos {
	install := token.NoPos
	mark := func(position token.Pos) {
		if install == token.NoPos || position < install {
			install = position
		}
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall && isSelector(call.Fun, "installPoint") {
			mark(call.Pos())
			return true
		}
		assignment, isAssignment := node.(*ast.AssignStmt)
		if !isAssignment {
			return true
		}
		for _, target := range assignment.Lhs {
			index, isIndex := target.(*ast.IndexExpr)
			if !isIndex || !isSelector(index.X, "points") {
				continue
			}
			mark(index.Pos())
		}
		return true
	})
	return install
}

// installPoint is the single cut into the published point plane. Nothing else
// in the package may assign into it, because a second install site is a
// second place to forget the evidence a publication owes its consumers.
func TestOnlyInstallPointWritesThePublishedPointPlane(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	installs := 0
	for _, parsed := range packages {
		for _, file := range parsed.Files {
			for _, declaration := range file.Decls {
				function, isFunction := declaration.(*ast.FuncDecl)
				if !isFunction || function.Body == nil {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					assignment, isAssignment := node.(*ast.AssignStmt)
					if !isAssignment {
						return true
					}
					for _, target := range assignment.Lhs {
						index, isIndex := target.(*ast.IndexExpr)
						if !isIndex || !isSelector(index.X, "points") {
							continue
						}
						installs++
						if function.Name.Name != "installPoint" {
							t.Errorf("%s: %s installs into the published point plane outside installPoint",
								fset.Position(index.Pos()), function.Name.Name)
						}
					}
					return true
				})
			}
		}
	}
	if installs != 1 {
		t.Fatalf("published point installs = %d want exactly one", installs)
	}
}

// auditPublicationReturns requires every post-install refusal to fail-stop.
func auditPublicationReturns(t *testing.T, fset *token.FileSet, function *ast.FuncDecl, install token.Pos) {
	t.Helper()
	ast.Inspect(function.Body, func(node ast.Node) bool {
		statement, isReturn := node.(*ast.ReturnStmt)
		if !isReturn || statement.Pos() < install || len(statement.Results) != 2 {
			return true
		}
		admitted := statement.Results[1]
		if identifier, isIdentifier := admitted.(*ast.Ident); isIdentifier && identifier.Name == "true" {
			return true
		}
		if isFailStop(admitted) {
			return true
		}
		t.Errorf("%s: %s returns without the publication fail-stop after installing a point state",
			fset.Position(statement.Pos()), function.Name.Name)
		return true
	})
}

func isFailStop(expression ast.Expr) bool {
	call, isCall := expression.(*ast.CallExpr)
	return isCall && len(call.Args) == 0 && isSelector(call.Fun, "fail")
}

func isSelector(expression ast.Expr, field string) bool {
	selector, isSelector := expression.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != field {
		return false
	}
	receiver, isName := selector.X.(*ast.Ident)
	return isName && receiver.Name == "epoch"
}

// The fail-stop must be visible to the liveness probe every operation
// consults, and it must stay invisible to the withdrawal-only probe that
// decides terminal status and failure recording.
func TestFailStopRefusesTheEpochWithoutClaimingCancellation(t *testing.T) {
	epoch := &executorEpoch{ctx: context.Background()}
	epoch.terminal.Store(epochRunning)
	if epoch.canceled() || epoch.canceledByContext() || !epoch.checkpoint() {
		t.Fatal("a running epoch reports as stopped")
	}
	if epoch.fail() {
		t.Fatal("fail returned an admission")
	}
	if !epoch.failed {
		t.Fatal("fail did not set the fail-stop")
	}
	if !epoch.canceled() {
		t.Fatal("a fail-stopped epoch is still live")
	}
	if epoch.canceledByContext() {
		t.Fatal("a fail-stopped epoch reports as withdrawn")
	}
	if epoch.checkpoint() {
		t.Fatal("a fail-stopped epoch still passes its checkpoint")
	}
	if epoch.terminal.Load() != epochRunning {
		t.Fatal("the fail-stop moved the terminal status")
	}
}
