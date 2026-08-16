// Package lower constructs canonical Programs from logical source inputs.
package lower

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	calllower "github.com/wippyai/go-lua/analysis/lua/lower/internal/call"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/control"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	functionlower "github.com/wippyai/go-lua/analysis/lua/lower/internal/function"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	modulelower "github.com/wippyai/go-lua/analysis/lua/lower/internal/module"
	sourcelower "github.com/wippyai/go-lua/analysis/lua/lower/internal/source"
	staticlower "github.com/wippyai/go-lua/analysis/lua/lower/internal/static"
	storagelower "github.com/wippyai/go-lua/analysis/lua/lower/internal/storage"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// Source is one logical source input to canonical Program construction. Name
// is the caller-supplied logical reflection name.
type Source struct {
	Name string
	Text []byte
}

// Lower parses, binds, lowers, and publishes exactly one logical source. Syntax
// without a canonical Program relation fails explicitly.
func Lower(source Source) (*program.Program, error) {
	if source.Name == "" {
		return nil, fmt.Errorf("lualower: source: empty Name")
	}
	statements, err := parse.Parse(bytes.NewReader(source.Text), source.Name)
	if err != nil {
		var parseError *parse.Error
		if errors.As(err, &parseError) {
			// Source.Text is caller-owned and the successful ingress path must
			// neither copy nor retain it. A parse diagnostic is the exceptional
			// path that needs an owned rendering copy.
			parseError.Source = string(source.Text)
		}
		return nil, fmt.Errorf("lualower: parse: %w", err)
	}
	binding := bind.BindChunk(statements)
	if binding == nil {
		return nil, fmt.Errorf("lualower: bind: no binding result")
	}
	result, err := lowerChunk(source.Name, statements, binding)
	if err != nil {
		return nil, fmt.Errorf("lualower: lower: %w", err)
	}
	return result, nil
}

func lowerChunk(sourceName string, statements []ast.Stmt, binding *bind.Result) (*program.Program, error) {
	if binding == nil {
		return nil, fmt.Errorf("lualower: nil binding result")
	}
	moduleCensus, err := modulelower.BuildCensus(binding)
	if err != nil {
		return nil, err
	}
	construction := assembly.New(sourceName, moduleCensus.Count(), binding.GlobalCensus())
	stack := new(continuation.Stack)
	expressions := continuation.NewExpressions(stack)
	bodies := continuation.NewBodies(stack)
	statics := continuation.NewStatics(stack)
	modules := modulelower.New(construction, moduleCensus)
	valuesValue := eval.New(stack, construction, expressions, statics)
	values := &valuesValue
	scopes := lexical.New(stack, construction, binding, sourceName, &modules, values, statics)
	static := staticlower.New(stack, construction, construction, binding, scopes, expressions, values, sourceName)
	storage := storagelower.New(stack, binding, scopes, values, expressions, static, construction, sourceName)
	controlsValue := control.New(
		construction, binding, scopes, values, stack, expressions, bodies, sourceName,
	)
	controls := &controlsValue
	calls := calllower.New(
		stack, expressions, values, storage, static, &modules,
		construction, binding, sourceName,
	)
	functionsValue := functionlower.New(
		stack, construction, binding, scopes, values, storage, static,
		expressions, bodies, statics, sourceName,
	)
	functions := &functionsValue
	tablesValue := storagelower.NewTable(stack, expressions, values, storage, sourceName)
	tables := &tablesValue

	source := sourcelower.New(
		sourceName, stack, expressions, bodies, statics, scopes, controls,
		values, storage, static, calls, functions, tables,
	)
	entry, err := source.Begin(statements)
	if err != nil {
		return nil, err
	}
	if err := source.Drain(); err != nil {
		return nil, err
	}
	closed, open := stack.Result()
	if closed != entry || open {
		return nil, fmt.Errorf("lualower: entry Body did not close exactly once")
	}
	if !source.Clean() || !modules.Clean() {
		return nil, fmt.Errorf("lualower: unfinished assembly state")
	}
	published, err := construction.Publish()
	if err != nil {
		return nil, fmt.Errorf("lualower: publish: %w", err)
	}
	return published, nil
}
