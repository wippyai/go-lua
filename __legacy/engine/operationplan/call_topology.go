package operationplan

import (
	"bytes"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type CallTopologyCandidate struct {
	Identity identity.ID
	Target   lexicalidentity.StableLexicalBodyID
}

type CallTopologySiteInput struct {
	Owner      lexicalidentity.StableLexicalBodyID
	Point      cfg.Point
	Candidates []CallTopologyCandidate
}

type CallTopologyComponentInput struct {
	Body      lexicalidentity.StableLexicalBodyID
	Component uint32
}

type CallTopologyBoundaryInput struct {
	Body            lexicalidentity.StableLexicalBodyID
	Captures        []symbol.ID
	Globals         []symbol.ID
	GlobalContracts []product.Value
}

type callTopologyBoundary struct {
	captures        []symbol.ID
	globals         []symbol.ID
	globalContracts []product.Value
}

type CallTopologySite struct {
	owner      lexicalidentity.StableLexicalBodyID
	point      cfg.Point
	candidates []CallTopologyCandidate
}

func (s CallTopologySite) Owner() lexicalidentity.StableLexicalBodyID { return s.owner }
func (s CallTopologySite) Point() cfg.Point                           { return s.point }
func (s CallTopologySite) Candidates() []CallTopologyCandidate {
	return append([]CallTopologyCandidate(nil), s.candidates...)
}
func (s CallTopologySite) Residual() bool { return len(s.candidates) != 0 }

// CallTopology is the sole immutable forest-wide call authority. Component
// zero means acyclic. Carrier inventories join this value at the same seal.
type CallTopology struct {
	sites      []CallTopologySite
	bodies     map[lexicalidentity.StableLexicalBodyID]struct{}
	components map[lexicalidentity.StableLexicalBodyID]uint32
	boundaries map[lexicalidentity.StableLexicalBodyID]callTopologyBoundary
	complete   bool
}

func SealCallTopology(bodies []lexicalidentity.StableLexicalBodyID, sites []CallTopologySiteInput, components []CallTopologyComponentInput, boundaries []CallTopologyBoundaryInput) (CallTopology, error) {
	out := CallTopology{bodies: make(map[lexicalidentity.StableLexicalBodyID]struct{}, len(bodies)), components: make(map[lexicalidentity.StableLexicalBodyID]uint32, len(components)), boundaries: make(map[lexicalidentity.StableLexicalBodyID]callTopologyBoundary, len(boundaries))}
	ownedBodies := append([]lexicalidentity.StableLexicalBodyID(nil), bodies...)
	sort.Slice(ownedBodies, func(i, j int) bool { return bytes.Compare(ownedBodies[i][:], ownedBodies[j][:]) < 0 })
	for index, body := range ownedBodies {
		if body == (lexicalidentity.StableLexicalBodyID{}) || index > 0 && body == ownedBodies[index-1] {
			return CallTopology{}, errors.New("operationplan: malformed call topology body inventory")
		}
		out.bodies[body] = struct{}{}
	}
	if len(out.bodies) == 0 {
		return CallTopology{}, errors.New("operationplan: empty call topology body inventory")
	}
	ownedSites := append([]CallTopologySiteInput(nil), sites...)
	sort.Slice(ownedSites, func(i, j int) bool {
		if cmp := bytes.Compare(ownedSites[i].Owner[:], ownedSites[j].Owner[:]); cmp != 0 {
			return cmp < 0
		}
		return ownedSites[i].Point < ownedSites[j].Point
	})
	for index, input := range ownedSites {
		_, ownerPresent := out.bodies[input.Owner]
		if !ownerPresent || len(input.Candidates) == 0 ||
			index > 0 && input.Owner == ownedSites[index-1].Owner && input.Point == ownedSites[index-1].Point {
			return CallTopology{}, errors.New("operationplan: malformed call topology site")
		}
		candidates := append([]CallTopologyCandidate(nil), input.Candidates...)
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Identity.Index < candidates[j].Identity.Index })
		seenIdentity := make(map[identity.ID]struct{}, len(candidates))
		seenTarget := make(map[lexicalidentity.StableLexicalBodyID]struct{}, len(candidates))
		for _, candidate := range candidates {
			_, targetPresent := out.bodies[candidate.Target]
			_, duplicateIdentity := seenIdentity[candidate.Identity]
			_, duplicateTarget := seenTarget[candidate.Target]
			if candidate.Identity.Kind != "lua.function" || candidate.Identity.Site != "symbol" || candidate.Identity.Index == 0 ||
				!targetPresent || duplicateIdentity || duplicateTarget {
				return CallTopology{}, errors.New("operationplan: malformed call topology candidates")
			}
			seenIdentity[candidate.Identity], seenTarget[candidate.Target] = struct{}{}, struct{}{}
		}
		out.sites = append(out.sites, CallTopologySite{owner: input.Owner, point: input.Point, candidates: candidates})
	}
	for _, input := range components {
		_, bodyPresent := out.bodies[input.Body]
		if !bodyPresent || input.Component == 0 {
			return CallTopology{}, errors.New("operationplan: malformed call topology component")
		}
		if _, duplicate := out.components[input.Body]; duplicate {
			return CallTopology{}, errors.New("operationplan: duplicate call topology component body")
		}
		out.components[input.Body] = input.Component
	}
	seenComponents := make(map[uint32]struct{})
	var maxComponent uint32
	for _, component := range out.components {
		seenComponents[component] = struct{}{}
		if component > maxComponent {
			maxComponent = component
		}
	}
	for component := uint32(1); component <= maxComponent; component++ {
		if _, present := seenComponents[component]; !present {
			return CallTopology{}, errors.New("operationplan: sparse call topology component IDs")
		}
	}
	for _, input := range boundaries {
		_, bodyPresent := out.bodies[input.Body]
		if !bodyPresent || len(input.Globals) != len(input.GlobalContracts) {
			return CallTopology{}, errors.New("operationplan: malformed call topology boundary")
		}
		if _, duplicate := out.boundaries[input.Body]; duplicate {
			return CallTopology{}, errors.New("operationplan: duplicate call topology boundary body")
		}
		seen := make(map[symbol.ID]struct{}, len(input.Captures)+len(input.Globals))
		for _, capture := range input.Captures {
			if capture == 0 {
				return CallTopology{}, errors.New("operationplan: zero call topology capture")
			}
			if _, exists := seen[capture]; exists {
				return CallTopology{}, errors.New("operationplan: duplicate call topology carrier")
			}
			seen[capture] = struct{}{}
		}
		for _, global := range input.Globals {
			if global == 0 {
				return CallTopology{}, errors.New("operationplan: zero call topology global")
			}
			if _, exists := seen[global]; exists {
				return CallTopology{}, errors.New("operationplan: duplicate call topology carrier")
			}
			seen[global] = struct{}{}
		}
		out.boundaries[input.Body] = callTopologyBoundary{captures: append([]symbol.ID(nil), input.Captures...), globals: append([]symbol.ID(nil), input.Globals...), globalContracts: append([]product.Value(nil), input.GlobalContracts...)}
	}
	if len(out.boundaries) != len(out.bodies) {
		return CallTopology{}, errors.New("operationplan: incomplete call topology boundaries")
	}
	out.complete = true
	return out, nil
}

func (t CallTopology) Complete() bool { return t.complete }
func (t CallTopology) Bodies() []lexicalidentity.StableLexicalBodyID {
	out := make([]lexicalidentity.StableLexicalBodyID, 0, len(t.bodies))
	for body := range t.bodies {
		out = append(out, body)
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i][:], out[j][:]) < 0 })
	return out
}
func (t CallTopology) Sites(owner lexicalidentity.StableLexicalBodyID) []CallTopologySite {
	start := sort.Search(len(t.sites), func(i int) bool { return bytes.Compare(t.sites[i].owner[:], owner[:]) >= 0 })
	var out []CallTopologySite
	for i := start; i < len(t.sites) && t.sites[i].owner == owner; i++ {
		site := t.sites[i]
		site.candidates = append([]CallTopologyCandidate(nil), site.candidates...)
		out = append(out, site)
	}
	return out
}
func (t CallTopology) Component(body lexicalidentity.StableLexicalBodyID) uint32 {
	return t.components[body]
}

func (t CallTopology) Boundary(body lexicalidentity.StableLexicalBodyID) (captures, globals []symbol.ID, contracts []product.Value, ok bool) {
	boundary, ok := t.boundaries[body]
	if !ok {
		return nil, nil, nil, false
	}
	return append([]symbol.ID(nil), boundary.captures...), append([]symbol.ID(nil), boundary.globals...), append([]product.Value(nil), boundary.globalContracts...), true
}
