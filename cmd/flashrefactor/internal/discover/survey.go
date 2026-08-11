package discover

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// Propose turns one semantic source snapshot into review-only closure facts.
// It intentionally does not fabricate destination identities or routes.
func Propose(input SurveyInput, snapshot semantic.Snapshot) (Proposal, error) {
	if input.Destination == "" || len(input.Symbols) == 0 {
		return Proposal{}, fmt.Errorf("survey needs selected symbols and a desired destination")
	}
	selected := map[string]bool{}
	for _, symbol := range input.Symbols {
		selected[symbol] = true
	}
	proposal := Proposal{Kind: "flashrefactor-survey-proposal-v1", Destination: input.Destination}
	reads, writes, closure, residue := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, object := range snapshot.Objects {
		if !selected[object.Object.Object] {
			continue
		}
		closure[object.Object.Object] = true
		residue[object.Object.Object] = true
		paths := map[string]bool{object.Definition.Path: true}
		for _, site := range object.References {
			paths[site.Path] = true
		}
		for path := range paths {
			reads[path] = true
			writes[path] = true
			proposal.BindingCandidates = append(proposal.BindingCandidates, path+" -> "+object.Object.Object)
		}
		proposal.Ambiguous = append(proposal.Ambiguous, "destination identity unresolved for "+object.Object.Object)
	}
	for _, symbol := range input.Containment {
		proposal.Ambiguous = append(proposal.Ambiguous, "containment relation requires reviewed target mapping for "+symbol)
	}
	for value := range closure {
		proposal.ReferenceClosure = append(proposal.ReferenceClosure, value)
	}
	for value := range residue {
		proposal.Residue = append(proposal.Residue, value)
	}
	for value := range reads {
		proposal.Read = append(proposal.Read, value)
	}
	for value := range writes {
		proposal.Write = append(proposal.Write, value)
	}
	sort.Strings(proposal.ReferenceClosure)
	sort.Strings(proposal.BindingCandidates)
	sort.Strings(proposal.ImportCandidates)
	sort.Strings(proposal.Read)
	sort.Strings(proposal.Write)
	sort.Strings(proposal.Residue)
	sort.Strings(proposal.Ambiguous)
	return proposal, nil
}
