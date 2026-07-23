package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	checkprojection "github.com/wippyai/go-lua/analysis/check/internal/projection"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
)

// DescriptorHandler lowers one executable operation into the existing Summary
// payload. Handlers mutate only a transaction-local summary.
type DescriptorHandler interface {
	Kind() DescriptorKind
	ConditionalAllowed() bool
	Apply(*axis.Registry, *summary.Summary, uint32, product.Value) error
}

// DescriptorRegistry is the only transformer-to-Summary specialization seam.
type DescriptorRegistry struct {
	handlers map[DescriptorKind]DescriptorHandler
	sealed   bool
}

func NewDescriptorRegistry(handlers ...DescriptorHandler) (*DescriptorRegistry, error) {
	r := &DescriptorRegistry{handlers: make(map[DescriptorKind]DescriptorHandler, len(handlers))}
	for _, handler := range handlers {
		if handler == nil {
			return nil, fmt.Errorf("transformer: nil descriptor handler")
		}
		kind := handler.Kind()
		if kind == "" {
			return nil, fmt.Errorf("transformer: empty descriptor kind")
		}
		if _, exists := r.handlers[kind]; exists {
			return nil, fmt.Errorf("transformer: duplicate descriptor handler %q", kind)
		}
		r.handlers[kind] = handler
	}
	return r, nil
}

func newDefaultDescriptorRegistry() *DescriptorRegistry {
	r, err := NewDescriptorRegistry(returnHandler{}, obligationHandler{})
	if err != nil {
		panic(err)
	}
	r.sealed = true
	return r
}

func newCompilerDescriptorRegistry(declared []product.Value) (*DescriptorRegistry, error) {
	r, err := NewDescriptorRegistry(returnHandler{declared: append([]product.Value(nil), declared...)}, obligationHandler{})
	if err != nil {
		return nil, err
	}
	r.sealed = true
	return r, nil
}

func (r *DescriptorRegistry) validSchema(reg *axis.Registry) bool {
	if r == nil || !r.sealed || len(r.handlers) != 2 {
		return false
	}
	returns, ok := r.handlers[DescriptorReturn].(returnHandler)
	if !ok {
		return false
	}
	if _, ok := r.handlers[DescriptorObligation].(obligationHandler); !ok {
		return false
	}
	for _, declared := range returns.declared {
		if !product.BelongsToRegistry(reg, declared) {
			return false
		}
	}
	return true
}

var defaultDescriptorRegistry = newDefaultDescriptorRegistry()

func DefaultDescriptorRegistry() *DescriptorRegistry { return defaultDescriptorRegistry }

type returnHandler struct{ declared []product.Value }

func (returnHandler) Kind() DescriptorKind     { return DescriptorReturn }
func (returnHandler) ConditionalAllowed() bool { return true }
func (h returnHandler) Apply(reg *axis.Registry, out *summary.Summary, slot uint32, value product.Value) error {
	if int(slot) < len(h.declared) {
		value = checkprojection.WithDeclaredContractPreservingPresence(reg, value, h.declared[slot])
	}
	priorLen := len(out.Returns)
	for len(out.Returns) <= int(slot) {
		out.Returns = append(out.Returns, product.Bottom(reg))
	}
	if int(slot) >= priorLen {
		out.Returns[slot] = value
		return nil
	}
	out.Returns[slot] = summary.JoinReturnValue(reg, out.Returns[slot], value)
	return nil
}

type obligationHandler struct{}

func (obligationHandler) Kind() DescriptorKind     { return DescriptorObligation }
func (obligationHandler) ConditionalAllowed() bool { return false }
func (obligationHandler) Apply(reg *axis.Registry, out *summary.Summary, slot uint32, value product.Value) error {
	priorLen := len(out.ParamObligations)
	for len(out.ParamObligations) <= int(slot) {
		out.ParamObligations = append(out.ParamObligations, product.Top())
	}
	if int(slot) >= priorLen {
		out.ParamObligations[slot] = value
	} else {
		out.ParamObligations[slot] = product.Meet(reg, out.ParamObligations[slot], value)
	}
	return nil
}

func (a *Arena) evalGuard(guard Guard, cursor BindingCursor, context SpecializationContext) (bool, bool) {
	if guard == 0 || int(guard) >= len(a.guards) || a.reg == nil {
		return false, false
	}
	n := a.guards[guard]
	switch n.op {
	case guardTrue:
		return true, true
	case guardFalse:
		return false, true
	case guardTruthy, guardFalsy:
		value, ok := a.evalValue(n.value, cursor, context)
		if !ok {
			return false, false
		}
		if n.op == guardTruthy {
			return valueref.CanBeTruthy(a.reg, value), true
		}
		return valueref.CanBeFalsy(a.reg, value), true
	case guardAnd:
		for _, arg := range n.args {
			holds, ok := a.evalGuard(arg, cursor, context)
			if !ok {
				return false, false
			}
			if !holds {
				return false, true
			}
		}
		return true, true
	case guardOr:
		for _, arg := range n.args {
			holds, ok := a.evalGuard(arg, cursor, context)
			if !ok {
				return false, false
			}
			if holds {
				return true, true
			}
		}
		return false, true
	default:
		return false, false
	}
}
