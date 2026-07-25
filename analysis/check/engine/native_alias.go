package engine

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// aliasNativeFacts projects only straight-line, allocation-owned alias facts.
// The WIR owns both allocation and assignment topology; paths that cross a
// branch or call are deliberately withheld rather than approximated.
func aliasNativeFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		rows = append(rows, aliasBodyFacts(compilation)...)
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	visit(root)
	return rows
}

func aliasBodyFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	if body == nil || aliasBodyHasControlOrCall(body) {
		return nil
	}
	type allocation struct {
		path path.Path
	}
	allocations := make(map[string]allocation)
	aliases := make(map[string]string)
	written := make(map[string]bool)
	var rows []NativeFact

	pathOperand := func(operand wir.Operand) (path.Path, bool) {
		if operand.Kind != wir.OperandPath {
			return path.Path{}, false
		}
		item := body.Path(wir.PathRef(operand.Ref))
		return item, !item.IsEmpty()
	}
	root := func(item path.Path) path.Path {
		item.Segments = nil
		item.Version = 0
		return item
	}
	key := func(item path.Path) string { return string(root(item).Key()) }
	term := func(item path.Path) string { return "path/" + string(item.Key()) }
	row := func(subject path.Path, occurrence, content string, revocations []NativeRevocation) NativeFact {
		fact := NativeFact{
			Lane: NativeLaneValues, Family: "alias_disjoint",
			Key:   "alias_disjoint/" + term(subject) + "/" + occurrence,
			Value: content, Term: term(subject), Subject: subject.String(), Occurrence: occurrence,
			Trust: NativeTrustProven, Revocations: revocations,
		}
		if len(revocations) != 0 {
			fact.Established = revocations[0].Established
			fact.Revoked = revocations[0].Revoked
			fact.Event = revocations[0].Event
		}
		return fact
	}
	contract := func(events ...string) []NativeRevocation {
		out := make([]NativeRevocation, 0, len(events))
		for _, event := range events {
			out = append(out, NativeRevocation{Established: "contract", Revoked: "contract/" + event, Event: event})
		}
		return out
	}

	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		occurrence := fmt.Sprintf("op-%08d", index)
		switch instruction.Op {
		case wir.OpMakeTable:
			destination, ok := pathOperand(instruction.Dst)
			if !ok || len(destination.Segments) != 0 {
				continue
			}
			rootKey := key(destination)
			prior := make([]allocation, 0, len(allocations))
			for priorKey, item := range allocations {
				if priorKey != rootKey {
					prior = append(prior, item)
				}
			}
			sort.Slice(prior, func(i, j int) bool { return prior[i].path.Key() < prior[j].path.Key() })
			for _, item := range prior {
				rows = append(rows, row(destination, occurrence,
					"against="+item.path.String()+" basis=distinct_fresh_allocations disjoint=true", contract("escape")))
			}
			allocations[rootKey] = allocation{path: destination}
			aliases[rootKey] = rootKey
		case wir.OpAssign:
			destination, destinationOK := pathOperand(instruction.Dst)
			source, sourceOK := pathOperand(instruction.A)
			if !destinationOK || !sourceOK {
				continue
			}
			if len(destination.Segments) == 0 && len(source.Segments) == 0 {
				if allocationKey, found := aliases[key(source)]; found {
					aliases[key(destination)] = allocationKey
					if allocationKey == key(source) && destination.Key() != source.Key() {
						rows = append(rows, row(destination, occurrence,
							"against="+source.String()+" basis=copy_of_same_binding disjoint=false", nil))
					}
				}
				continue
			}
			allocationKey := aliases[key(root(source))]
			if len(source.Segments) == 0 || allocationKey == "" || written[allocationKey] {
				continue
			}
			rows = append(rows, row(source, occurrence,
				"basis=no_intervening_store disjoint=true", contract("write.field", "escape", "call.opaque")))
		case wir.OpStaticMemberWrite, wir.OpDynamicIndexWrite:
			destination, ok := pathOperand(instruction.Dst)
			if ok {
				if allocationKey := aliases[key(root(destination))]; allocationKey != "" {
					written[allocationKey] = true
				}
			}
		}
	}
	return rows
}

func aliasBodyHasControlOrCall(body *wir.Body) bool {
	for index := 0; index < body.Len(); index++ {
		switch body.Instr(index).Op {
		case wir.OpBranch, wir.OpCall, wir.OpSelect, wir.OpIterate:
			return true
		}
	}
	return false
}
