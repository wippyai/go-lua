package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// The authoring shape shared by every Lua library instance in this package. A
// library contract is the same artifact whichever library it describes - a root
// export, the callables reachable from it, what the contract can say about their
// results, and the metatable edge a library may publish - so the shape is stated
// once and each library file carries only its own world.
//
// The typed application of each callable is authored here, beside the inventory
// that names it. A contract instance is the authority for what it says about the
// values it attaches to; a signature read out of another table at authoring time
// would make that table the authority and this instance its projection, which is
// the addressing the whole surface exists to leave behind. The modeled
// standard-library table still exists, and while it does the drift law derives
// what THAT table must hold from these instances - the direction that makes the
// instance the source and the table the checked side.

// librarySpec is one authored Lua library: the mount selector it is chosen by,
// its export inventory, the typed application of each export, the result
// refinements it publishes, the exports whose result selection a rule owns, and
// the metatable key it publishes its members through.
type librarySpec struct {
	// Root is the authored mount selector.
	Root string
	// Exports is the authored export inventory, in canonical order. Each name is
	// one direct export of the contract root.
	Exports []string
	// Signatures is the authored typed application of each export, keyed by the
	// export name. An export with no authored signature leaves the contract
	// unauthored rather than published with a member it cannot describe.
	Signatures map[string]signature.Function
	// Aggregate is what the contract says about its root export: the aggregate a
	// mounted library is, and the mutability that aggregate is published with.
	Aggregate contract.ExportValue
	// Values are the exported values that are neither the root nor a callable,
	// in authored order: the constants a library publishes and the nested
	// aggregates its export graph continues through.
	Values []valueExport
	// Methods are the callables published at an address deeper than a direct
	// export of the root, in authored order. A member of an exported aggregate
	// is addressed by walking the export graph one step further, which is the
	// whole difference between it and an export - not a second inventory.
	Methods []methodExport
	// Refinements are the result refinements the contract publishes, keyed by
	// the export they refine.
	Refinements map[string]wire.ResultRefinement
	// Suspensions are the exports that suspend, keyed by the export they attach
	// to: the outcome case control leaves the member at, the outcome case it
	// re-enters at, and the policy that restores it.
	Suspensions map[string]contract.Suspension
	// Delegations are the exports whose result selection is driven by a caller
	// literal, which no enumeration can carry, so the contract names the rule
	// that owns the computation instead.
	Delegations []string
	// Denials are the exports the library declares and refuses to publish. A
	// member the contract describes and will not hand out is member data the
	// library owns, so the contract states the refusal instead of leaving every
	// consumer to encode its own exclusion list.
	Denials []string
	// MetatableIndex is the metatable key this library publishes its members
	// through, empty when the library publishes no metatable edge.
	MetatableIndex string
}

// valueExport is one exported value the contract does not describe as a
// callable: where it is reached from the contract root, and what it is. A
// constant terminates the path it is reached by; an aggregate is a value the
// contract keeps addressing through, so a deeper member may name it as a prefix.
type valueExport struct {
	Path  contract.Path
	Value contract.ExportValue
}

// methodExport is one callable published at an address deeper than a direct
// export of the contract root, with the typed application it is published under.
type methodExport struct {
	Path      contract.Path
	Signature signature.Function
}

// constantExport publishes one constant under a direct export key.
func constantExport(key string, constant contract.Constant, mutability contract.Mutability) valueExport {
	return valueExport{
		Path:  contract.Export(key),
		Value: contract.ExportValue{Shape: contract.ValueShapeConstant, Mutability: mutability, Constant: constant},
	}
}

// aggregateExport publishes one nested aggregate at the address it is reached by.
func aggregateExport(path contract.Path, mutability contract.Mutability) valueExport {
	return valueExport{Path: path, Value: contract.Aggregate(mutability)}
}

// instance authors one library contract instance against one declared kind. The
// kind is the authority for the codec, the payload format identity of every
// member form and the addressing law; nothing here restates any of them, so a
// kind that changed its algebra rejects this instance instead of silently
// admitting members it can no longer describe.
//
// Member order is the authored order: the root export, the other exported
// values, the metatable edge, the callables exported from the root, the
// callables published deeper in the export graph, the refinements, the
// suspensions, the delegations, and the denials. Order is content - the
// instance identity is the digest of exactly these bytes - so it is stated here
// once rather than per library.
func (spec librarySpec) instance(kind *library.Entry) (*contract.Instance, bool) {
	if kind == nil || kind.Class() != library.ClassLibrary || spec.Root == "" {
		return nil, false
	}
	members := make([]contract.Member, 0, len(spec.Exports)+len(spec.Values)+len(spec.Methods)+
		len(spec.Delegations)+len(spec.Refinements)+len(spec.Denials)+2)
	aggregate, err := contract.EncodeExportValue(spec.Aggregate)
	if err != nil {
		return nil, false
	}
	members = append(members, resolvedMember(kind, library.FormExportValue, contract.Root(), aggregate))
	for _, value := range spec.Values {
		if value.Path.Len() == 0 {
			return nil, false
		}
		body, err := contract.EncodeExportValue(value.Value)
		if err != nil {
			return nil, false
		}
		members = append(members, resolvedMember(kind, library.FormExportValue, value.Path, body))
	}
	if spec.MetatableIndex != "" {
		edge, err := contract.EncodePath(contract.Root())
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormMetatableEdge, contract.Metatable(spec.MetatableIndex), edge))
	}
	for _, name := range spec.Exports {
		envelope, envelopeOK := callableEnvelope(spec.Signatures[name])
		if !envelopeOK {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormCallableSignature, contract.Export(name), envelope))
	}
	for _, method := range spec.Methods {
		if method.Path.Len() < 2 {
			return nil, false
		}
		envelope, envelopeOK := callableEnvelope(method.Signature)
		if !envelopeOK {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormCallableSignature, method.Path, envelope))
	}
	// The refinements are walked in inventory order rather than in map order, so
	// what the instance publishes is the authored artifact and not a traversal.
	for _, name := range spec.Exports {
		refinement, published := spec.Refinements[name]
		if !published {
			continue
		}
		body, err := wire.EncodeResultRefinement(refinement)
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormResultRefinement, contract.Export(name), body))
	}
	// A suspension is walked in inventory order for the same reason, and it says
	// what it can say today: which sealed outcome case the member leaves control
	// at, which it re-enters at, and under which authority.
	for _, name := range spec.Exports {
		suspension, published := spec.Suspensions[name]
		if !published {
			continue
		}
		body, err := contract.EncodeSuspension(suspension)
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormSuspension, contract.Export(name), body))
	}
	for _, name := range spec.Delegations {
		members = append(members, member(kind, library.FormRuleDelegation, contract.Export(name)))
	}
	// A library denial is a member the library models and will not hand out, so
	// its payload is that member's address and the refusal it states. A library
	// never states the other refusal the form can carry: whether a member is
	// there at all is what the host booted, and a library that claimed a member
	// was absent would be speaking for the environment that mounted it.
	for _, name := range spec.Denials {
		body, err := contract.EncodeDeniedEntry(contract.DeniedEntry{
			Denial: contract.DenialRefused,
			Entry:  contract.Export(name),
		})
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormDeniedEntry, contract.Export(name), body))
	}
	return contract.New(contract.Spec{
		Kind:    kind.Key(),
		Codec:   kind.Codec(),
		Root:    spec.Root,
		Members: members,
	}, kind)
}

// callableEnvelope writes the typed application envelope of one callable. An
// export with no authored signature, or a signature the envelope format cannot
// carry, leaves the contract unauthored: a callable member with no envelope
// would claim an application it cannot state.
func callableEnvelope(sig signature.Function) ([]byte, bool) {
	if sig.Type == nil {
		return nil, false
	}
	body, err := wire.EncodeCallableSignature(signature.Function{Type: sig.Type, Effect: sig.Effect})
	if err != nil {
		return nil, false
	}
	return body, true
}

// authored is the authoring form of one exported callable: the typed
// application, and the audited effect labels its application exercises. It is
// the one place a library instance in this package builds a signature, so an
// export's effect row is written the same way whichever library owns it.
func authored(fn *typ.Function, labels ...effect.Label) signature.Function {
	return signature.Function{Type: fn, Effect: effect.Row{Labels: labels}}
}

// member authors one deferred row. An unresolvable payload identity leaves the
// row's identity empty, which admission rejects: a member the kind does not
// declare a format for is a member no reader could interpret.
func member(kind *library.Entry, form library.Form, path contract.Path) contract.Member {
	payload, _ := kind.Payload(form)
	return contract.Member{Form: form, Path: path, Payload: payload, Encoding: contract.EncodingDeferred}
}

func resolvedMember(kind *library.Entry, form library.Form, path contract.Path, body []byte) contract.Member {
	row := member(kind, form, path)
	row.Encoding, row.Body = contract.EncodingResolved, body
	return row
}

// copyNames returns a copy of one authored inventory, so a reader cannot rewrite
// an authored contract through the slice it was handed.
func copyNames(names []string) []string { return append([]string(nil), names...) }
