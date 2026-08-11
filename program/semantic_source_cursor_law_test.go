package program_test

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"

	programflow "github.com/wippyai/go-lua/program/flow"
	programmodule "github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/semanticsource"
	programsource "github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
	"github.com/wippyai/go-lua/program/testfixture"
)

func TestProgramChildSemanticSourceCursorsHaveExactOwnerRanges(t *testing.T) {
	p, err := testfixture.Minimal()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}

	sourceView, err := programsource.SemanticSourceFragmentView(p.Source())
	if err != nil {
		t.Fatalf("Source fragment view: %v", err)
	}
	flowView, err := programflow.SemanticSourceFragmentView(p.Flow())
	if err != nil {
		t.Fatalf("Flow fragment view: %v", err)
	}
	staticView, err := programstatic.SemanticSourceFragmentView(p.Static())
	if err != nil {
		t.Fatalf("Static fragment view: %v", err)
	}
	moduleView, err := programmodule.SemanticSourceFragmentView(p.Module())
	if err != nil {
		t.Fatalf("Module fragment view: %v", err)
	}

	assertChildRange(t, "Source", sourceView.Count(), 8, sourceView.At, func() programsource.SemanticSourceCursor { return sourceView.Cursor() })
	assertChildRange(t, "Flow", flowView.Count(), 33, flowView.At, func() programflow.SemanticSourceCursor { return flowView.Cursor() })
	assertChildRange(t, "Static", staticView.Count(), 10, staticView.At, func() programstatic.SemanticSourceCursor { return staticView.Cursor() })
	assertChildRange(t, "Module", moduleView.Count(), 6, moduleView.At, func() programmodule.SemanticSourceCursor { return moduleView.Cursor() })

	if sourceView.OwnerID() != p.Source().Identity().ContentID() || flowView.OwnerID() != p.Flow().ContentID() ||
		staticView.OwnerID() != p.Static().ContentID() || moduleView.OwnerID() != p.Module().ContentID() {
		t.Fatal("child semantic-source cursor crossed an owner identity")
	}

	// The fixed rows retain required zero claims instead of dropping them from
	// the owner denominator.
	zero := 0
	for _, row := range sourceView.Publications() {
		if row.Count() == 0 {
			zero++
		}
	}
	for _, row := range flowView.Publications() {
		if row.Count() == 0 {
			zero++
		}
	}
	for _, row := range staticView.Publications() {
		if row.Count() == 0 {
			zero++
		}
	}
	for _, row := range moduleView.Publications() {
		if row.Count() == 0 {
			zero++
		}
	}
	if zero == 0 {
		t.Fatal("child cursor ranges discarded all zero-count claims")
	}
	receipt, ok := p.SemanticSourceReceipt()
	if !ok {
		t.Fatal("Program semantic-source receipt unavailable")
	}
	views, ok := receipt.Views()
	if !ok || views.Source().Count()+views.Flow().Count()+views.Static().Count()+views.Module().Count() != receipt.Count() || receipt.Count() != 57 {
		t.Fatalf("aggregate child partition = %d/%d/%d/%d, receipt=%d; want 8/33/10/6 and 57", views.Source().Count(), views.Flow().Count(), views.Static().Count(), views.Module().Count(), receipt.Count())
	}
}

func assertChildRange[T any](t *testing.T, name string, got, want int, at func(int) (semanticsource.Publication, bool), cursor func() T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s Count = %d, want %d", name, got, want)
	}
	for index := 0; index < want; index++ {
		if _, ok := at(index); !ok {
			t.Fatalf("%s At(%d) rejected an in-range row", name, index)
		}
	}
	if _, ok := at(want); ok {
		t.Fatalf("%s At(Count) accepted a row", name)
	}
	// The type-specific cursor is deliberately exercised through a small
	// interface so every owner gets the same exact traversal law.
	type nextCursor interface {
		Next() (semanticsource.Publication, bool)
	}
	value := cursor()
	walker, ok := any(&value).(nextCursor)
	if !ok {
		t.Fatalf("%s cursor does not expose Next", name)
	}
	for index := 0; index < want; index++ {
		if _, ok := walker.Next(); !ok {
			t.Fatalf("%s cursor ended at %d", name, index)
		}
	}
	if _, ok := walker.Next(); ok {
		t.Fatalf("%s cursor yielded past Count", name)
	}
}

func TestProgramChildSemanticSourceSealsRejectMalformedRangesAndReplay(t *testing.T) {
	p, err := testfixture.Minimal()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	sourceRows, err := programsource.SemanticSourceFragment(p.Source())
	if err != nil {
		t.Fatal(err)
	}
	flowRows, err := programflow.SemanticSourceFragment(p.Flow())
	if err != nil {
		t.Fatal(err)
	}
	staticRows, err := programstatic.SemanticSourceFragment(p.Static())
	if err != nil {
		t.Fatal(err)
	}
	moduleRows, err := programmodule.SemanticSourceFragment(p.Module())
	if err != nil {
		t.Fatal(err)
	}

	assertMalformedSource := func(name string, seal func([]semanticsource.Publication) error, rows []semanticsource.Publication, foreign semanticsource.Publication) {
		t.Helper()
		if len(rows) < 2 {
			t.Fatalf("%s fixture range too short", name)
		}
		reordered := append([]semanticsource.Publication(nil), rows...)
		reordered[0], reordered[1] = reordered[1], reordered[0]
		if err := seal(reordered); !errors.Is(err, semanticsource.ErrPublicationOrder) {
			t.Fatalf("%s reordered error = %v, want publication order", name, err)
		}
		duplicate := append([]semanticsource.Publication(nil), rows...)
		duplicate[1] = duplicate[0]
		if err := seal(duplicate); !errors.Is(err, semanticsource.ErrDuplicatePublication) {
			t.Fatalf("%s duplicate error = %v, want duplicate", name, err)
		}
		missing := append([]semanticsource.Publication(nil), rows[:len(rows)-1]...)
		if err := seal(missing); !errors.Is(err, semanticsource.ErrMissingPublication) {
			t.Fatalf("%s missing error = %v, want missing", name, err)
		}
		foreignRows := append([]semanticsource.Publication(nil), rows...)
		foreignRows[0] = foreign
		if err := seal(foreignRows); !errors.Is(err, semanticsource.ErrUnexpectedPublication) {
			t.Fatalf("%s foreign error = %v, want unexpected", name, err)
		}
	}

	assertMalformedSource("Source", func(rows []semanticsource.Publication) error {
		_, err := programsource.SealSemanticSourceFragment(p.Source().Identity().ContentID(), rows)
		return err
	}, sourceRows, flowRows[0])
	assertMalformedSource("Flow", func(rows []semanticsource.Publication) error {
		_, err := programflow.SealSemanticSourceFragment(p.Flow().ContentID(), rows)
		return err
	}, flowRows, sourceRows[0])
	assertMalformedSource("Static", func(rows []semanticsource.Publication) error {
		_, err := programstatic.SealSemanticSourceFragment(p.Static().ContentID(), rows)
		return err
	}, staticRows, sourceRows[0])
	assertMalformedSource("Module", func(rows []semanticsource.Publication) error {
		_, err := programmodule.SealSemanticSourceFragment(p.Module().ContentID(), rows)
		return err
	}, moduleRows, sourceRows[0])

	// Replay over detached rows has a stable digest and cannot be changed by a
	// caller mutating a returned publication slice.
	first, err := programsource.SemanticSourceFragmentView(p.Source())
	if err != nil {
		t.Fatal(err)
	}
	second, err := programsource.SemanticSourceFragmentView(p.Source())
	if err != nil {
		t.Fatal(err)
	}
	if digestPublicationRange(first.Publications()) != digestPublicationRange(second.Publications()) {
		t.Fatal("Source publication replay changed its digest")
	}
	rows := first.Publications()
	rows[0] = semanticsource.Publication{}
	if digestPublicationRange(first.Publications()) == digestPublicationRange(rows) {
		t.Fatal("Source publication copy aliases sealed rows")
	}
}

func digestPublicationRange(rows []semanticsource.Publication) [32]byte {
	hash := sha256.New()
	var frame [24]byte
	for _, row := range rows {
		token := row.Definition().Token()
		binary.BigEndian.PutUint32(frame[0:4], uint32(token.Origin()))
		binary.BigEndian.PutUint16(frame[4:6], uint16(token.Facet()))
		binary.BigEndian.PutUint16(frame[6:8], uint16(token.Revision()))
		binary.BigEndian.PutUint64(frame[8:16], token.Digest())
		binary.BigEndian.PutUint64(frame[16:24], uint64(row.Count()))
		_, _ = hash.Write(frame[:])
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}
