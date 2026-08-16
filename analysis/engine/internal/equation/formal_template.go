package equation

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// FormalTemplate is the equation-owned sealed formal receipt for one
// activation endpoint. Its Batch, formal ports, and ordinary target rows are
// admitted and sealed together; callers can only bind it to a concrete Batch.
type FormalTemplate struct {
	batch *Batch
	ports map[composition.Key]FormalPort
	sites []Site
	// roleCount is the sealed ABI denominator. The authored Template is not
	// retained after issuance; all structural rows live in batch.targets.
	roleCount   int
	prototypes  []formalPrototypeReceipt
	points      map[PointRef]Point
	portModes   map[composition.Key]PortMode
	portReads   map[composition.Key][]PortRead
	selectors   []prototypeSelectorSurface
	exports     []composition.Key
	factorEdges int
}

// formalPrototypeReceipt is the opaque rule-row projection issued at the same
// seal as the formal Batch. It is the only parent authority accepted by typed
// payload attachment; canonical instance construction never escapes this
// receipt boundary.
type formalPrototypeReceipt struct {
	key composition.Key
	row RuleInstance
}

func NewFormalTemplate(value Template) (FormalTemplate, bool) {
	result, batch, ports, ok := normalizeTemplateFormal(value)
	if !ok || batch == nil || !batch.Sealed() {
		return FormalTemplate{}, false
	}
	return formalTemplateFromParts(batch, ports, len(result.Roles)), true
}

func formalTemplateFromParts(batch *Batch, ports map[composition.Key]FormalPort, roleCount int) FormalTemplate {
	sites := make([]Site, len(batch.sites))
	for index := range batch.sites {
		sites[index] = Site{batch: batch, row: uint32(index + 1)}
		if !sites[index].Available() {
			return FormalTemplate{}
		}
	}
	return FormalTemplate{batch: batch, ports: ports, sites: sites, roleCount: roleCount}
}

func (value FormalTemplate) withProjections(instances []canonicalInstance, points map[PointRef]Point, modes map[composition.Key]PortMode, reads map[composition.Key][]PortRead, selectors []prototypeSelectorSurface, exports []composition.Key, factorEdges int) FormalTemplate {
	value.prototypes = make([]formalPrototypeReceipt, len(instances))
	for index, instance := range instances {
		value.prototypes[index] = formalPrototypeReceipt{key: instance.key, row: copyInstance(instance.row)}
	}
	value.points = cloneFormalPoints(points)
	value.portModes = clonePortModes(modes)
	value.portReads = clonePortReads(reads)
	value.selectors = append([]prototypeSelectorSurface(nil), selectors...)
	value.exports = append([]composition.Key(nil), exports...)
	value.factorEdges = factorEdges
	return value
}

func cloneFormalPoints(values map[PointRef]Point) map[PointRef]Point {
	result := make(map[PointRef]Point, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func (plan VariantPlan) FormalTemplate(target, endpoint composition.Key) (FormalTemplate, bool) {
	variant, ok := plan.variant(target, endpoint)
	if !ok || !variant.template.formal.Available() {
		return FormalTemplate{}, false
	}
	return variant.template.formal, true
}

func (value FormalTemplate) Available() bool {
	return value.batch != nil && value.batch.Sealed() && len(value.sites) == len(value.batch.sites) && len(value.ports) == value.roleCount
}

func (value FormalTemplate) Batch() *Batch {
	if !value.Available() {
		return nil
	}
	return value.batch
}

func (value FormalTemplate) Port(role composition.Key) (FormalPort, bool) {
	if !value.Available() || !role.Available() {
		return FormalPort{}, false
	}
	port, ok := value.ports[role]
	return port, ok && port.Available()
}

func (value FormalTemplate) Materialize(source *composition.Composition, actuals *Batch, values []FormalPortActual) (TemplateMaterialization, bool) {
	if !value.Available() || source == nil || actuals == nil {
		return TemplateMaterialization{}, false
	}
	for _, row := range values {
		if !row.Role.Available() || row.Role.batch != value.batch {
			return TemplateMaterialization{}, false
		}
		if expected, ok := value.ports[row.Role.Role()]; !ok || !expected.Same(row.Role) {
			return TemplateMaterialization{}, false
		}
	}
	binding, ok := SealTemplateBinding(value.batch, actuals, values)
	if !ok {
		return TemplateMaterialization{}, false
	}
	return MaterializeTemplateBoundary(source, binding, value.sites, nil)
}
