package effect

import "fmt"

type Borrow struct {
	Param ParamRef
}

func (Borrow) label() {}
func (b Borrow) String() string {
	return fmt.Sprintf("borrow(%s)", b.Param)
}
func (b Borrow) Equals(other Label) bool {
	if o, ok := other.(Borrow); ok {
		return b.Param.Index == o.Param.Index
	}
	return false
}

type Store struct {
	Param ParamRef
	Into  ParamRef
}

func (Store) label() {}
func (s Store) String() string {
	if s.Into.Index >= 0 {
		return fmt.Sprintf("store(%s into %s)", s.Param, s.Into)
	}
	return fmt.Sprintf("store(%s)", s.Param)
}
func (s Store) Equals(other Label) bool {
	if o, ok := other.(Store); ok {
		return s.Param.Index == o.Param.Index && s.Into.Index == o.Into.Index
	}
	return false
}

type BorrowAll struct{}

func (BorrowAll) label()         {}
func (BorrowAll) String() string { return "borrow_all" }
func (BorrowAll) Equals(other Label) bool {
	_, ok := other.(BorrowAll)
	return ok
}

type Send struct {
	FromParam int
}

func (Send) label() {}
func (s Send) String() string {
	return fmt.Sprintf("send(params[%d:])", s.FromParam)
}
func (s Send) Equals(other Label) bool {
	if o, ok := other.(Send); ok {
		return s.FromParam == o.FromParam
	}
	return false
}

type Freeze struct {
	Param ParamRef
}

func (Freeze) label() {}
func (f Freeze) String() string {
	return fmt.Sprintf("freeze(%s)", f.Param)
}
func (f Freeze) Equals(other Label) bool {
	if o, ok := other.(Freeze); ok {
		return f.Param.Index == o.Param.Index
	}
	return false
}
