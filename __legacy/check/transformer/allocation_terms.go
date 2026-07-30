package transformer

import (
	"fmt"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// formalObjectMaterializationTemplates assigns every object graph effect an
// owner-local allocation coordinate after the Arena's signature allocations.
// EffectTerm and object ordinals are dense sealed program structure, so this
// is the same finite existential namespace consumed by the existing root/apply
// allocation quotient; no concrete object identity or solve state enters the
// coordinate.
func formalObjectMaterializationTemplates(relation Relation, effect EffectTerm) ([]identity.AllocationTemplate, error) {
	if relation.arena == nil || relation.arena.owner == (lexicalidentity.StableLexicalBodyID{}) || relation.effects == nil ||
		!relation.effects.Sealed() || effect == 0 || int(effect) >= len(relation.effects.nodes) ||
		(relation.effects.nodes[effect].kind != EffectObjectMaterialization && relation.effects.nodes[effect].kind != EffectPathStore) {
		return nil, fmt.Errorf("transformer: formal object allocation is unowned")
	}
	base := uint64(len(relation.arena.allocations) - 1)
	allocation := base + uint64(effect)
	node := relation.effects.nodes[effect]
	if allocation == 0 || allocation > math.MaxUint32 || len(node.pathStoreObject.Heaps) > math.MaxUint32 {
		return nil, fmt.Errorf("transformer: formal object allocation exceeds finite coordinate space")
	}
	out := make([]identity.AllocationTemplate, len(node.pathStoreObject.Heaps))
	for objectIndex := range out {
		out[objectIndex] = identity.ManifestAllocationTemplate(relation.arena.owner, uint32(allocation), uint32(objectIndex+1))
		if !out[objectIndex].Valid() {
			return nil, fmt.Errorf("transformer: formal object allocation coordinate %d is invalid", objectIndex)
		}
	}
	return out, nil
}

func relationObjectMaterializationTemplates(relation Relation) ([]identity.AllocationTemplate, error) {
	if relation.effects == nil || !relation.effects.Sealed() {
		return nil, fmt.Errorf("transformer: relation object allocation inventory is unsealed")
	}
	var out []identity.AllocationTemplate
	for effect := EffectTerm(1); int(effect) < len(relation.effects.nodes); effect++ {
		node := relation.effects.nodes[effect]
		if node.kind != EffectObjectMaterialization && (node.kind != EffectPathStore || len(node.pathStoreObject.Heaps) == 0) {
			continue
		}
		templates, err := formalObjectMaterializationTemplates(relation, effect)
		if err != nil {
			return nil, err
		}
		out = append(out, templates...)
	}
	return out, nil
}

// canonicalAllocationTemplates assigns every object in one sealed allocation
// operation a stable, dense structural coordinate. Signature names validate
// graph references but never become recursive allocation identity input.
func canonicalAllocationTemplates(owner lexicalidentity.StableLexicalBodyID, allocation AllocationTemplateTerm, op operationplan.SignatureAllocationOperation) (map[signature.AllocationTemplateID]identity.AllocationTemplate, error) {
	if owner == (lexicalidentity.StableLexicalBodyID{}) || allocation == 0 {
		return nil, fmt.Errorf("transformer: allocation template has no lexical owner")
	}
	template := op.Template()
	objects := append([]signature.AllocationObjectTemplate(nil), template.Objects...)
	sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })
	out := make(map[signature.AllocationTemplateID]identity.AllocationTemplate, len(objects))
	for index, object := range objects {
		if object.ID == "" || index > 0 && object.ID == objects[index-1].ID {
			return nil, fmt.Errorf("transformer: allocation template has zero or duplicate object identity")
		}
		coordinate := identity.ManifestAllocationTemplate(owner, uint32(allocation), uint32(index+1))
		if !coordinate.Valid() {
			return nil, fmt.Errorf("transformer: allocation template coordinate is invalid")
		}
		out[object.ID] = coordinate
	}
	if _, ok := out[template.Root]; !ok {
		return nil, fmt.Errorf("transformer: allocation template root is absent")
	}
	return out, nil
}

// AllocationTemplateTerm is one correlated symbolic allocation transaction.
// Result ValueTerms and the heap/fresh EffectTerm reference this same node.
type AllocationTemplateTerm uint32

type allocationTemplateNode struct {
	op         operationplan.SignatureAllocationOperation
	templates  []identity.AllocationTemplate
	identities map[signature.AllocationTemplateID]identity.Term
}

func (a *Arena) AllocationTemplate(op operationplan.SignatureAllocationOperation) AllocationTemplateTerm {
	if a == nil || op.Site().Owner == 0 || op.Site().Ordinal == 0 || op.Site().Template == "" {
		return 0
	}
	site := op.Site()
	key := a.maskFingerprint(allocationTemplateFingerprint(site.Owner, string(site.Template), site.Ordinal))
	for _, term := range a.allocationKeys[key] {
		if allocationOperationEqual(a.allocations[term].op, op) {
			return term
		}
	}
	// Lexical owner binding freezes the complete structural allocation
	// inventory. Relation freezing may reuse existing terms but cannot discover
	// a new allocation operation after coordinates have been assigned.
	if a.sealed || a.owner != (lexicalidentity.StableLexicalBodyID{}) {
		return 0
	}
	term := AllocationTemplateTerm(len(a.allocations))
	a.allocations = append(a.allocations, allocationTemplateNode{op: op})
	a.allocationKeys[key] = append(a.allocationKeys[key], term)
	return term
}

func (a *Arena) AllocationResultValue(allocation AllocationTemplateTerm, resultIndex int) ValueTerm {
	if !a.validAllocation(allocation) || resultIndex < 0 || resultIndex != a.allocations[allocation].op.Template().ReturnIndex {
		return 0
	}
	return a.internValue(valueNode{op: valueAllocationResult, allocation: allocation, resultIndex: resultIndex})
}

func (a *Arena) validAllocation(term AllocationTemplateTerm) bool {
	return a != nil && term != 0 && int(term) < len(a.allocations)
}

func (a *Arena) allocationResult(term AllocationTemplateTerm, resultIndex int) (product.Value, bool) {
	if a == nil || a.reg == nil || a.owner == (lexicalidentity.StableLexicalBodyID{}) || !a.validAllocation(term) {
		return product.Value{}, false
	}
	op := a.allocations[term].op
	template := op.Template()
	if resultIndex != template.ReturnIndex {
		return product.Value{}, false
	}
	node := a.allocations[term]
	if len(node.identities) == 0 || len(node.templates) == 0 {
		return product.Value{}, false
	}
	return effectlowering.StaticAllocationResult(a.reg, nil, template, node.identities)
}

func allocationOperationEqual(a, b operationplan.SignatureAllocationOperation) bool {
	if a.Site() != b.Site() {
		return false
	}
	return (signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{a.Template()}}).Equals(
		signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{b.Template()}},
	)
}
