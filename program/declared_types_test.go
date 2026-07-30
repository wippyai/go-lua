package program

import "testing"

func TestDeclaredCellTypeQueriesAndDirectLookup(t *testing.T) {
	b, entry := staticEntry(t)
	first, second := b.Cell(Span{}, entry), b.Cell(Span{}, entry)
	target := b.Primitive(Span{}, PrimitiveNumber)
	declared := b.DeclareCellType(Span{}, first, target)
	values := b.Values(Span{}, entry, []Term{b.Nil(Span{}, entry), b.Nil(Span{}, entry)}, 0)
	bind := b.Bind(Span{}, entry, []Term{first, second}, values)
	if first == 0 || second == 0 || target == 0 || declared == 0 || values == 0 || bind == 0 || !b.SetBody(entry, bind) {
		t.Fatal("build declared Cell type")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if count := p.DeclaredTypeCount(); count != 1 {
		t.Fatalf("declared type count=%d", count)
	}
	if got, ok := p.DeclaredTypeAt(0); !ok || got != declared {
		t.Fatalf("declared type at=%v/%v", got, ok)
	}
	if host, gotTarget, ok := p.DeclaredType(declared); !ok || host != first || gotTarget != target {
		t.Fatalf("declared type=%v/%v/%v", host, gotTarget, ok)
	}
	if got, ok := p.CellDeclaredType(first); !ok || got != declared {
		t.Fatalf("first lookup=%v/%v", got, ok)
	}
	if got, ok := p.CellDeclaredType(second); ok || got != 0 {
		t.Fatalf("second lookup=%v/%v", got, ok)
	}
}

func TestDeclaredCellTypeRejectsDuplicateCellAttachment(t *testing.T) {
	b, entry := staticEntry(t)
	cell := b.Cell(Span{}, entry)
	first, second := b.Primitive(Span{}, PrimitiveNumber), b.Primitive(Span{}, PrimitiveString)
	if cell == 0 || first == 0 || second == 0 || b.DeclareCellType(Span{}, cell, first) == 0 || b.DeclareCellType(Span{}, cell, second) == 0 {
		t.Fatal("build duplicate declared Cell type")
	}
	values := b.Values(Span{}, entry, []Term{b.Nil(Span{}, entry)}, 0)
	bind := b.Bind(Span{}, entry, []Term{cell}, values)
	if bind == 0 || !b.SetBody(entry, bind) {
		t.Fatal("bind duplicate Cell")
	}
	if _, err := b.Seal(); err == nil {
		t.Fatal("Seal accepted duplicate declared Cell type")
	}
}
