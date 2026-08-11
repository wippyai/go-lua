package frame

import "testing"

func TestFrameAccessClosureBroadSparseScale(t *testing.T) {
	const roots = 65_537
	equalities := make([]Equality, 0, roots/16)
	follows := make([]Follow, 0, roots+roots/31)
	for root := 1; root < roots; root++ {
		follows = append(follows, Follow{From: Root(root), To: Root(root + 1)})
		if root%31 == 0 {
			follows = append(follows, Follow{From: Root(root), To: Root(root + 1)})
		}
	}
	for root := 2; root+1 < roots; root += 16 {
		equalities = append(equalities, Equality{Left: Root(root), Right: Root(root + 1)})
	}
	closure, ok := Compile(Spec{
		Roots:      roots,
		Equalities: equalities,
		Follows:    follows,
		Projections: []Projection{{
			Known:    true,
			MayRead:  []Root{1},
			MayWrite: []Root{roots},
		}},
	})
	if !ok || !closure.Valid() || !closure.Known() {
		t.Fatal("sparse closure compilation")
	}
	for root := Root(1); root <= roots; root++ {
		if !closure.MayRead(root) {
			t.Fatalf("forward sparse closure missed Root %d", root)
		}
	}
	if !closure.MayWrite(roots) || closure.MayWrite(1) {
		t.Fatal("write closure lost directed sparsity")
	}
}
