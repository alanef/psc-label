package main

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
)

// Label geometry. The printed area is 60mm x 40mm; the page is 3mm larger in
// each direction to allow for the printer's non-printing margin. Changing
// these means re-checking the printer setup instructions in the template.
const (
	labelWidth  = 63.0
	labelHeight = 43.0

	marginLeft = 5.0
	marginTop  = 3.0
	// marginRight is the same as the printer's right-hand dead zone.
	marginRight = 3.0

	// maxNameRunes is where a long name gets truncated to keep it on one line.
	maxNameRunes = 16
)

// fpdfDoc is an alias so the rest of the program does not need to import the
// PDF library directly — swapping libraries stays a change to this file.
type fpdfDoc = fpdf.Fpdf

// newLabelPDF returns an empty document. Each label is its own page.
func newLabelPDF() *fpdfDoc {
	return fpdf.New("P", "mm", "A4", "")
}

// addLabel renders one licence label: a black banner with the year, then the
// member's name, boat and berth.
func addLabel(pdf *fpdfDoc, year int, name, boat, berth string) {
	startLabelPage(pdf)

	pdf.SetFont("Arial", "B", 18)
	pdf.SetFillColor(0, 0, 0)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(0, 12, fmt.Sprintf("PSC Licence %4d", year), "1", 0, "C", true, 0, "")
	pdf.Ln(12)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Arial", "B", 15)
	pdf.MultiCell(0, 8, fmt.Sprintf("%s\n%s\n%s", truncate(name, maxNameRunes), boat, berth), "1", "L", false)
}

// addNumberLabel renders a single large number, used for berth markers.
func addNumberLabel(pdf *fpdfDoc, number int) {
	startLabelPage(pdf)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Arial", "B", 58)
	pdf.CellFormat(0, 37, fmt.Sprintf("%d", number), "1", 0, "C", false, 0, "")
}

func startLabelPage(pdf *fpdfDoc) {
	pdf.AddPageFormat("P", fpdf.SizeType{Wd: labelWidth, Ht: labelHeight})
	pdf.SetMargins(marginLeft, marginTop, marginRight)
	pdf.SetAutoPageBreak(false, 1)
	pdf.SetXY(marginLeft, marginTop)
}

// renderPDF returns the finished document as bytes.
//
// The original wrote a PDF into the working directory and served it as a static
// file. That made it dependent on a writable CWD and, on Windows, on the file
// not being locked by the browser's PDF viewer — either of which killed the
// process. Nothing touches the disk now.
func renderPDF(pdf *fpdfDoc) ([]byte, error) {
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// truncate shortens s to at most n runes (not bytes, so accented names survive).
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}
