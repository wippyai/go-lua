package channelruntime

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect/channelselect"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestIsSelectCallRecognizesAmbientGlobalOnly(t *testing.T) {
	t.Run("exact global form", func(t *testing.T) {
		call, bindings := mustSelectCall(t, `
type Message = {kind: string}
local ch: Channel<Message>
local result = channel.select { ch:case_receive() }
`, "channel")
		if !IsSelectCall(call, bindings) {
			t.Fatalf("IsSelectCall rejected ambient channel.select")
		}
	})

	t.Run("rejects wrong shapes", func(t *testing.T) {
		call, bindings := mustSelectCall(t, `
type Message = {kind: string}
local ch: Channel<Message>
local result = channel.select { ch:case_receive() }
`, "channel")

		cases := []struct {
			name   string
			mutate func(*ast.FuncCallExpr)
		}{
			{
				name: "wrong arity",
				mutate: func(call *ast.FuncCallExpr) {
					call.Args = nil
				},
			},
			{
				name: "wrong receiver",
				mutate: func(call *ast.FuncCallExpr) {
					call.Receiver = ident("receiver")
				},
			},
			{
				name: "wrong method",
				mutate: func(call *ast.FuncCallExpr) {
					call.Method = "select"
				},
			},
			{
				name: "wrong type args",
				mutate: func(call *ast.FuncCallExpr) {
					call.TypeArgs = []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}}
				},
			},
		}

		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				mutated := *call
				tt.mutate(&mutated)
				if IsSelectCall(&mutated, bindings) {
					t.Fatalf("IsSelectCall accepted %s", tt.name)
				}
			})
		}
	})

	t.Run("rejects local shadow", func(t *testing.T) {
		call, bindings := mustSelectCall(t, `
type Message = {kind: string}
local channel = {}
local ch: Channel<Message>
local result = channel.select { ch:case_receive() }
`)
		if IsSelectCall(call, bindings) {
			t.Fatalf("IsSelectCall accepted local shadow of channel")
		}
	})
}

func TestIsReceiveCaseCallRequiresZeroArgsAndChannelReceiver(t *testing.T) {
	t.Run("exact channel receiver", func(t *testing.T) {
		call, bindings := mustReceiveCaseCall(t, `
type Message = {kind: string}
local ch: Channel<Message>
local result = ch:case_receive()
`, "ch")
		if !IsReceiveCaseCall(call, bindings) {
			t.Fatalf("IsReceiveCaseCall rejected ambient Channel<T>:case_receive()")
		}
	})

	t.Run("rejects wrong shapes", func(t *testing.T) {
		call, bindings := mustReceiveCaseCall(t, `
type Message = {kind: string}
local ch: Channel<Message>
local result = ch:case_receive()
`, "ch")

		cases := []struct {
			name   string
			mutate func(*ast.FuncCallExpr)
		}{
			{
				name: "wrong args",
				mutate: func(call *ast.FuncCallExpr) {
					call.Args = []ast.Expr{ident("extra")}
				},
			},
			{
				name: "wrong type args",
				mutate: func(call *ast.FuncCallExpr) {
					call.TypeArgs = []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}}
				},
			},
			{
				name: "wrong method",
				mutate: func(call *ast.FuncCallExpr) {
					call.Method = "receive"
				},
			},
			{
				name: "wrong receiver type",
				mutate: func(call *ast.FuncCallExpr) {
					call.Receiver = ident("obj")
				},
			},
		}

		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				mutated := *call
				tt.mutate(&mutated)
				if IsReceiveCaseCall(&mutated, bindings) {
					t.Fatalf("IsReceiveCaseCall accepted %s", tt.name)
				}
			})
		}
	})
}

func TestPathTypeAndChannelPayloadTypeFollowAnnotatedPathsAndAliases(t *testing.T) {
	stmts, bindings := mustParsedChunk(t, `
type Message = {kind: string}
type MessageChannel = Channel<Message>
local alias_ch: MessageChannel
local box: {inner: MessageChannel}
`)

	aliasStmt := stmts[2].(*ast.LocalAssignStmt)
	boxStmt := stmts[3].(*ast.LocalAssignStmt)

	wantPayload := typetableRecord(t)

	aliasPath := path.NewPath(mustLocalAt(t, bindings, aliasStmt, 0), "alias_ch")
	aliasType, ok := pathType(bindings, aliasPath)
	if !ok {
		t.Fatalf("pathType(alias) failed")
	}
	if !isChannelType(aliasType) {
		t.Fatalf("pathType(alias) = %v, want Channel<T>", aliasType)
	}
	payload, ok := channelselect.ChannelPayloadType(aliasType)
	if !ok || !typ.TypeEquals(payload, wantPayload) {
		t.Fatalf("ChannelPayloadType(alias) = %v/%v, want %v", payload, ok, wantPayload)
	}

	boxPath := path.NewPath(mustLocalAt(t, bindings, boxStmt, 0), "box").Field("inner")
	boxType, ok := pathType(bindings, boxPath)
	if !ok {
		t.Fatalf("pathType(projected) failed")
	}
	if !isChannelType(boxType) {
		t.Fatalf("pathType(projected) = %v, want Channel<T>", boxType)
	}
	payload, ok = channelselect.ChannelPayloadType(boxType)
	if !ok || !typ.TypeEquals(payload, wantPayload) {
		t.Fatalf("ChannelPayloadType(projected) = %v/%v, want %v", payload, ok, wantPayload)
	}

	aliased := typ.NewAlias("MessageChannelAlias", typ.Instantiate(ambient.ChannelGeneric(), wantPayload))
	payload, ok = channelselect.ChannelPayloadType(aliased)
	if !ok || !typ.TypeEquals(payload, wantPayload) {
		t.Fatalf("ChannelPayloadType(alias wrapper) = %v/%v, want %v", payload, ok, wantPayload)
	}
}

func mustSelectCall(t *testing.T, src string, globals ...string) (*ast.FuncCallExpr, *bind.Result) {
	t.Helper()
	stmts, bindings := mustParsedChunk(t, src, globals...)
	stmt := stmts[len(stmts)-1]
	local, ok := stmt.(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("last stmt = %T, want *ast.LocalAssignStmt", stmt)
	}
	call, ok := local.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("local expr = %T, want *ast.FuncCallExpr", local.Exprs[0])
	}
	return call, bindings
}

func mustReceiveCaseCall(t *testing.T, src string, globals ...string) (*ast.FuncCallExpr, *bind.Result) {
	t.Helper()
	return mustSelectCaseCall(t, src, globals...)
}

func mustSelectCaseCall(t *testing.T, src string, globals ...string) (*ast.FuncCallExpr, *bind.Result) {
	t.Helper()
	stmts, bindings := mustParsedChunk(t, src, globals...)
	stmt := stmts[len(stmts)-1]
	local, ok := stmt.(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("last stmt = %T, want *ast.LocalAssignStmt", stmt)
	}
	call, ok := local.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("local expr = %T, want *ast.FuncCallExpr", local.Exprs[0])
	}
	return call, bindings
}

func mustParsedChunk(t *testing.T, src string, globals ...string) ([]ast.Stmt, *bind.Result) {
	t.Helper()
	stmts, err := parse.ParseString(src, "test")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	return stmts, bindings
}

func mustLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	id, ok := bindings.LocalSymbolAt(stmt, index)
	if !ok {
		t.Fatalf("missing local symbol at %d", index)
	}
	return id
}

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func typetableRecord(t *testing.T) typ.Type {
	t.Helper()
	return table.NewRecord().Field("kind", typ.String).Build()
}
