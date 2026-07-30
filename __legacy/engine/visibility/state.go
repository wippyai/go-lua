package visibility

import (
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// A versionState is an immutable, structurally shared symbol-to-version index.
// Pages are a representation fanout, not a semantic bound: the page slice grows
// without limit and an overflowing page splits exactly.
const versionPageWidth = 32

type versionEntry struct {
	symbol  symbol.ID
	version ssa.Version
}

type versionPage struct {
	entries []versionEntry
}

type versionState struct {
	pages []*versionPage
	count int
}

func (s *versionState) lookup(sym symbol.ID) ssa.Version {
	if s == nil || sym == 0 || len(s.pages) == 0 {
		return ssa.Version{}
	}
	pageIndex := sort.Search(len(s.pages), func(i int) bool {
		entries := s.pages[i].entries
		return entries[len(entries)-1].symbol >= sym
	})
	if pageIndex == len(s.pages) {
		return ssa.Version{}
	}
	entries := s.pages[pageIndex].entries
	entryIndex := sort.Search(len(entries), func(i int) bool { return entries[i].symbol >= sym })
	if entryIndex == len(entries) || entries[entryIndex].symbol != sym {
		return ssa.Version{}
	}
	return entries[entryIndex].version
}

func (s *versionState) with(version ssa.Version) *versionState {
	if version.Symbol == 0 || version.ID <= 0 {
		return s
	}
	if s == nil || len(s.pages) == 0 {
		return &versionState{
			pages: []*versionPage{{entries: []versionEntry{{symbol: version.Symbol, version: version}}}},
			count: 1,
		}
	}

	pageIndex := sort.Search(len(s.pages), func(i int) bool {
		entries := s.pages[i].entries
		return entries[len(entries)-1].symbol >= version.Symbol
	})
	if pageIndex == len(s.pages) {
		pageIndex--
	}
	page := s.pages[pageIndex]
	entryIndex := sort.Search(len(page.entries), func(i int) bool {
		return page.entries[i].symbol >= version.Symbol
	})
	if entryIndex < len(page.entries) && page.entries[entryIndex].symbol == version.Symbol {
		if versionSemanticallyEqual(page.entries[entryIndex].version, version) {
			return s
		}
		entries := append([]versionEntry(nil), page.entries...)
		entries[entryIndex].version = version
		pages := append([]*versionPage(nil), s.pages...)
		pages[pageIndex] = &versionPage{entries: entries}
		return &versionState{pages: pages, count: s.count}
	}

	entries := make([]versionEntry, len(page.entries)+1)
	copy(entries, page.entries[:entryIndex])
	entries[entryIndex] = versionEntry{symbol: version.Symbol, version: version}
	copy(entries[entryIndex+1:], page.entries[entryIndex:])
	pages := make([]*versionPage, 0, len(s.pages)+1)
	pages = append(pages, s.pages[:pageIndex]...)
	if len(entries) <= versionPageWidth*2 {
		pages = append(pages, &versionPage{entries: entries})
	} else {
		mid := len(entries) / 2
		left := append([]versionEntry(nil), entries[:mid]...)
		right := append([]versionEntry(nil), entries[mid:]...)
		pages = append(pages, &versionPage{entries: left}, &versionPage{entries: right})
	}
	pages = append(pages, s.pages[pageIndex+1:]...)
	return &versionState{pages: pages, count: s.count + 1}
}

func (s *versionState) forEach(fn func(symbol.ID, ssa.Version)) {
	if s == nil || fn == nil {
		return
	}
	for _, page := range s.pages {
		for _, entry := range page.entries {
			fn(entry.symbol, entry.version)
		}
	}
}

func versionStatesEqual(left, right *versionState) bool {
	if left == right {
		return true
	}
	if left == nil || right == nil || left.count != right.count {
		return false
	}
	leftPage, leftEntry := 0, 0
	rightPage, rightEntry := 0, 0
	for leftPage < len(left.pages) && rightPage < len(right.pages) {
		leftValue := left.pages[leftPage].entries[leftEntry]
		rightValue := right.pages[rightPage].entries[rightEntry]
		if leftValue.symbol != rightValue.symbol || !versionSemanticallyEqual(leftValue.version, rightValue.version) {
			return false
		}
		leftEntry++
		if leftEntry == len(left.pages[leftPage].entries) {
			leftPage++
			leftEntry = 0
		}
		rightEntry++
		if rightEntry == len(right.pages[rightPage].entries) {
			rightPage++
			rightEntry = 0
		}
	}
	return leftPage == len(left.pages) && rightPage == len(right.pages)
}

type versionStateBuilder struct {
	pages   []*versionPage
	current []versionEntry
	count   int
}

func (b *versionStateBuilder) append(sym symbol.ID, version ssa.Version) {
	if sym == 0 || version.ID <= 0 {
		return
	}
	version.Symbol = sym
	b.current = append(b.current, versionEntry{symbol: sym, version: version})
	b.count++
	if len(b.current) == versionPageWidth {
		b.flush()
	}
}

func (b *versionStateBuilder) flush() {
	if len(b.current) == 0 {
		return
	}
	b.pages = append(b.pages, &versionPage{entries: b.current})
	b.current = nil
}

func (b *versionStateBuilder) build() *versionState {
	b.flush()
	if b.count == 0 {
		return nil
	}
	return &versionState{pages: b.pages, count: b.count}
}
