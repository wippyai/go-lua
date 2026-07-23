package front

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/parse"
)

// ErrUnsupportedInstruction reports a WIR operation outside the front's
// admitted family. CompileBody never omits such an operation.
var ErrUnsupportedInstruction = errors.New("front: unsupported WIR instruction")

const (
	entryKernel = "front/entry/v1"
	entryName   = "entry"
)

// CompileBody parses source and lowers its chunk through bind, cfgbuild, and
// wirlower before compiling the resulting complete equation source. The
// walking skeleton admits only the structural entry operation; later families
// are added explicitly rather than being skipped.
func CompileBody(source string) (equation.Artifact, error) {
	stmts, err := parse.ParseString(source, "<front>")
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: parse body: %w", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		return equation.Artifact{}, fmt.Errorf("front: build CFG")
	}
	body := wirlower.Lower("chunk", stmts, bindings, built)
	if body == nil {
		return equation.Artifact{}, fmt.Errorf("front: lower WIR")
	}
	return compileWIR(source, body)
}

func compileWIR(source string, body *wir.Body) (equation.Artifact, error) {
	if body == nil {
		return equation.Artifact{}, fmt.Errorf("front: nil WIR body")
	}
	bodyID := bodyID(source)
	entry := equation.EntryParameter{Body: bodyID, Name: entryName}
	drafts := make([]equation.Draft, 0, 1)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpEntry:
			drafts = append(drafts, equation.Draft{
				Target:     equation.Coordinate{Body: bodyID, Name: operationName(len(drafts))},
				Entry:      entry,
				Occurrence: occurrence("entry"),
				Operands:   []equation.Operand{{Role: "entry", Term: equation.EntryTerm(entry)}},
			})
		case wir.OpExit, wir.OpNoop:
			// These WIR operations carry no transfer occurrence. They are CFG
			// structure, so retaining them as equations would invent semantics.
		default:
			return equation.Artifact{}, fmt.Errorf("%w: %d at instruction %d", ErrUnsupportedInstruction, instruction.Op, index)
		}
	}
	if len(drafts) != 1 {
		return equation.Artifact{}, fmt.Errorf("front: WIR body has no entry operation")
	}
	compiler, err := equation.Skeleton().With("entry", equation.BindExistingKernel(entryKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure entry compiler: %w", err)
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: drafts})
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: compile equations: %w", err)
	}
	return artifact, nil
}

func bodyID(source string) equation.BodyID {
	return equation.BodyID(sha256.Sum256(append([]byte("front/lua-body/v1\x00"), []byte(source)...)))
}

func occurrence(kind string) equation.Occurrence {
	return equation.Occurrence{Kind: kind, ContractID: equation.ContentID(sha256.Sum256([]byte("front/contract/v1/" + kind)))}
}

func operationName(index int) string { return fmt.Sprintf("op/%08d", index) }
