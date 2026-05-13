// Package pdfgen builds the printable session sheets:
// the handicap matrix and the score card.
package pdfgen

import (
	"fmt"
	"io"

	"github.com/go-pdf/fpdf"

	"github.com/levino/go-ranking/internal/rating"
	"github.com/levino/go-ranking/internal/store"
)

// MatrixOptions controls Matrix output.
type MatrixOptions struct {
	GroupName  string
	Passphrase string
	Date       string
	Boards     []rating.BoardSize // boards to print, in order
}

// Matrix writes a handicap matrix PDF for the given session snapshot
// to w.  Only active players (those present in the snapshot) appear.
func Matrix(w io.Writer, snap []store.SnapshotEntry, opt MatrixOptions) error {
	pdf := newPDF()
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for _, b := range opt.Boards {
		pdf.AddPage()
		drawHeader(pdf, tr, opt.GroupName, opt.Passphrase, opt.Date,
			fmt.Sprintf("Vorgabe-Matrix %s", b))
		drawMatrixTable(pdf, tr, snap, b)
		drawLegend(pdf, tr, snap)
	}
	return pdf.Output(w)
}

// newPDF creates a configured A4 PDF. Centralized so matrix and
// scorecard use identical layout settings.
func newPDF() *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 12)
	return pdf
}

// translator is the function type returned by fpdf's UnicodeTranslator.
// fpdf renders strings in cp1252 by default — we run all user-facing
// text through this to keep umlauts and ß looking like umlauts and ß.
type translator func(string) string

func drawHeader(pdf *fpdf.Fpdf, tr translator, group, pass, date, title string) {
	pdf.SetFont("Helvetica", "B", 18)
	pdf.CellFormat(0, 9, tr(title), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 6, tr(fmt.Sprintf("Gruppe: %s", group)), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 22)
	pdf.CellFormat(0, 12, tr(pass), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 6, tr(fmt.Sprintf("Datum: %s", date)), "", 1, "L", false, 0, "")
	pdf.Ln(2)
}

func drawMatrixTable(pdf *fpdf.Fpdf, tr translator, snap []store.SnapshotEntry, board rating.BoardSize) {
	n := len(snap)
	if n == 0 {
		pdf.SetFont("Helvetica", "I", 11)
		pdf.CellFormat(0, 6, tr("Keine aktiven Spieler in dieser Session."), "", 1, "L", false, 0, "")
		return
	}
	headW := 16.0
	colW := (180.0 - headW) / float64(n)
	if colW < 12 {
		colW = 12
	}
	if colW > 22 {
		colW = 22
	}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(headW, 8, tr("S \\ W"), "1", 0, "C", false, 0, "")
	for _, e := range snap {
		pdf.CellFormat(colW, 8, fmt.Sprintf("%d", e.Number), "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 8)
	for _, b := range snap {
		pdf.CellFormat(headW, 10, fmt.Sprintf("%d", b.Number), "1", 0, "C", false, 0, "")
		for _, w := range snap {
			if b.PlayerID == w.PlayerID {
				pdf.SetFillColor(220, 220, 220)
				pdf.CellFormat(colW, 10, tr("—"), "1", 0, "C", true, 0, "")
				pdf.SetFillColor(255, 255, 255)
				continue
			}
			cell := matrixCell(b.GoR, w.GoR, board)
			pdf.CellFormat(colW, 10, cell, "1", 0, "C", false, 0, "")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.CellFormat(0, 5,
		tr("Zeile = Schwarz, Spalte = Weiß. Anzeige: Vorgabe / Komi. "+
			"Bei Vorgabe spielt der stärkere Spieler Weiß."), "", 1, "L", false, 0, "")
}

func matrixCell(blackGor, whiteGor float64, board rating.BoardSize) string {
	stronger, weaker := whiteGor, blackGor
	if blackGor > whiteGor {
		stronger, weaker = blackGor, whiteGor
	}
	h := rating.Recommended(stronger, weaker, board)
	if h.Stones == 0 {
		return fmt.Sprintf("0 / %.1f", h.Komi)
	}
	return fmt.Sprintf("%d / %.1f", h.Stones, h.Komi)
}

func drawLegend(pdf *fpdf.Fpdf, tr translator, snap []store.SnapshotEntry) {
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 6, tr("Spieler-Legende"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	for _, e := range snap {
		pdf.CellFormat(0, 5,
			tr(fmt.Sprintf("%2d  %s  (GoR %.0f, %s)", e.Number, e.Name, e.GoR, rating.FormatRank(e.GoR))),
			"", 1, "L", false, 0, "")
	}
}
