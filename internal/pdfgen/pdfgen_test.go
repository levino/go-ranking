package pdfgen

import (
	"bytes"
	"testing"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/store"
)

func someSnapshot() []store.SnapshotEntry {
	return []store.SnapshotEntry{
		{PlayerID: 1, Number: 1, Name: "Alice", GoR: 1200},
		{PlayerID: 2, Number: 2, Name: "Bob", GoR: 800},
		{PlayerID: 3, Number: 3, Name: "Carl", GoR: 400},
	}
}

func TestMatrixProducesPDF(t *testing.T) {
	var buf bytes.Buffer
	err := Matrix(&buf, someSnapshot(), MatrixOptions{
		GroupName:  "Test",
		Passphrase: "happy-fox",
		Date:       "29.04.2026",
		Boards:     []rating.BoardSize{rating.Board9, rating.Board13},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("output is not PDF: %q", buf.Bytes()[:8])
	}
	if buf.Len() < 1500 {
		t.Errorf("unexpectedly small PDF: %d bytes", buf.Len())
	}
}

func TestScoreCardProducesPDF(t *testing.T) {
	var buf bytes.Buffer
	err := ScoreCard(&buf, someSnapshot(), ScoreCardOptions{
		GroupName:  "Test",
		Passphrase: "happy-fox",
		Date:       "29.04.2026",
		Rows:       12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("output is not PDF")
	}
}

func TestMatrixHandlesEmptySnapshot(t *testing.T) {
	var buf bytes.Buffer
	if err := Matrix(&buf, nil, MatrixOptions{
		GroupName: "Empty", Passphrase: "x-y", Date: "1.1.",
		Boards: []rating.BoardSize{rating.Board9},
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatal("must still produce a PDF")
	}
}
