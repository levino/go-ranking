package pdfgen

import (
	"fmt"
	"io"

	"github.com/go-pdf/fpdf"

	"github.com/levino/go-ranking/internal/store"
)

// ScoreCardOptions controls ScoreCard output.
type ScoreCardOptions struct {
	GroupName  string
	Passphrase string
	Date       string
	Rows       int // number of rows on the form (default 12)
}

// ScoreCard writes a printable score card. Each row has cells for
// black/white player numbers, board size (with options to strike out)
// and winner (with options to strike out).
func ScoreCard(w io.Writer, snap []store.SnapshotEntry, opt ScoreCardOptions) error {
	if opt.Rows == 0 {
		opt.Rows = 12
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()

	drawHeader(pdf, opt.GroupName, opt.Passphrase, opt.Date, "Ergebniszettel")

	// Column widths sum to 186mm (A4 minus 12mm margins each side).
	colS, colW, colB, colWin := 22.0, 22.0, 60.0, 82.0

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(colS, 8, "Schwarz", "1", 0, "C", false, 0, "")
	pdf.CellFormat(colW, 8, "Weiß", "1", 0, "C", false, 0, "")
	pdf.CellFormat(colB, 8, "Brett", "1", 0, "C", false, 0, "")
	pdf.CellFormat(colWin, 8, "Gewinner", "1", 1, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 13)
	for i := 0; i < opt.Rows; i++ {
		pdf.CellFormat(colS, 12, "", "1", 0, "C", false, 0, "")
		pdf.CellFormat(colW, 12, "", "1", 0, "C", false, 0, "")
		pdf.CellFormat(colB, 12, "9x9   /   13x13   /   19x19", "1", 0, "C", false, 0, "")
		pdf.CellFormat(colWin, 12, "Schwarz   /   Weiß", "1", 1, "C", false, 0, "")
	}

	pdf.Ln(4)
	pdf.SetFont("Helvetica", "I", 9)
	pdf.MultiCell(0, 4,
		"Tragen Sie pro Zeile die Spielernummern (siehe Vorgabe-Matrix) ein "+
			"und streichen Sie die nicht zutreffenden Optionen für Brettgröße "+
			"und Gewinner durch.", "", "L", false)

	pdf.Ln(2)
	drawLegend(pdf, snap)

	return pdf.Output(w)
}

// SummaryLine produces a one-line filename-friendly description.
func SummaryLine(opt ScoreCardOptions) string {
	return fmt.Sprintf("score-%s-%s", opt.GroupName, opt.Passphrase)
}
