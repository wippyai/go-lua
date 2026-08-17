package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) runAliasConstraints(current walkStep) error {
	if current.alias == nil || current.index < 0 || current.index > len(current.typeParams) {
		return fmt.Errorf("lualower: invalid type parameter cursor")
	}
	if current.index == len(current.typeParams) {
		w.push(walkStep{kind: finishAliasWalk, alias: current.alias, body: current.body, span: current.span})
		return w.scheduleType(current.alias.Type, current.typeHost, current.body, w.typeSpan(current.alias.Type))
	}
	param := current.typeParams[current.index]
	if param.ID == 0 || param.Kind != bind.TypeDeclParam {
		return fmt.Errorf("lualower: invalid type parameter binding")
	}
	current.index++
	if param.Constraint == nil {
		if err := w.FinishParam(param, 0); err != nil {
			return err
		}
		w.push(current)
		return nil
	}
	host, ok := w.Host(param)
	if !ok {
		return fmt.Errorf("lualower: type parameter was not predeclared")
	}
	w.push(current)
	w.push(walkStep{kind: finishParamWalk, typeParam: param, body: current.body, span: current.span})
	return w.scheduleType(param.Constraint, host, current.body, w.typeSpan(param.Constraint))
}

func (w *Writer) finishAlias(current walkStep) error {
	if current.alias == nil {
		return fmt.Errorf("lualower: invalid type alias completion")
	}
	return w.FinishAlias(current.alias, w.result())
}

func (w *Writer) runInterfaceExtends(current walkStep) error {
	if current.iface == nil || current.index < 0 || current.index > len(current.iface.Extends) {
		return fmt.Errorf("lualower: invalid interface extends cursor")
	}
	if current.index == len(current.iface.Extends) {
		w.push(walkStep{kind: interfaceMembersWalk, iface: current.iface, typeBase: current.typeBase, staticMark: current.staticMark, mark: current.mark, body: current.body, span: current.span})
		return nil
	}
	extend := current.iface.Extends[current.index]
	if extend == nil {
		return fmt.Errorf("lualower: absent interface extends reference at index %d", current.index)
	}
	current.index++
	w.push(current)
	w.push(walkStep{kind: appendTypeWalk, body: current.body, span: current.span})
	return w.scheduleType(extend, current.typeBase, current.body, w.typeSpan(extend))
}

func (w *Writer) runInterfaceMembers(current walkStep) error {
	if current.iface == nil || current.index < 0 || current.index > len(current.iface.Members) {
		return fmt.Errorf("lualower: invalid interface member cursor")
	}
	if current.index == len(current.iface.Members) {
		return w.FinishInterface(current.iface, current.staticMark, current.mark)
	}
	member := current.iface.Members[current.index]
	current.index++
	switch member.Kind {
	case ast.InterfaceFieldMember:
		if member.Name == "" || member.Type == nil {
			return fmt.Errorf("lualower: invalid interface field %d", current.index-1)
		}
		w.push(current)
		w.push(walkStep{kind: finishInterfaceFieldWalk, member: member, typeHost: current.typeBase, body: current.body, span: current.span})
		return w.scheduleType(member.Type, current.typeBase, current.body, w.typeSpan(member.Type))
	case ast.InterfaceMethodMember:
		if member.Name == "" {
			return fmt.Errorf("lualower: invalid interface method %d", current.index-1)
		}
		if _, ok := member.Type.(*ast.FunctionTypeExpr); !ok {
			return fmt.Errorf("lualower: interface method %q has non-function signature", member.Name)
		}
		w.push(current)
		w.push(walkStep{kind: appendInterfaceMethodWalk, member: member, body: current.body, span: current.span})
		return w.scheduleType(member.Type, current.typeBase, current.body, w.typeSpan(member.Type))
	default:
		return fmt.Errorf("lualower: invalid interface member kind %d", member.Kind)
	}
}
