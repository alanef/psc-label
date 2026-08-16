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

	// Number labels. 58pt suits four digits across the printable width; a
	// prefix or suffix makes the string longer, so the size is reduced until it
	// fits rather than letting it run off the label. numberFontMin is the point
	// at which a berth marker stops being readable across the yard.
	numberFontMax = 58.0
	numberFontMin = 14.0
)

// printableWidth is the space between the margins — what any label text has to
// fit inside.
const printableWidth = labelWidth - marginLeft - marginRight

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

// addNumberLabel renders a single large number, used for berth markers. An
// optional prefix and suffix bracket it, so a range can print as CT-123 or
// 123-XX rather than a bare number.
func addNumberLabel(pdf *fpdfDoc, prefix string, number int, suffix string) {
	startLabelPage(pdf)

	text := numberLabelText(prefix, number, suffix)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Arial", "B", fitNumberFont(pdf, text))
	pdf.CellFormat(0, 37, text, "1", 0, "C", false, 0, "")
}

// numberLabelText assembles the label. The prefix and suffix are taken as
// typed, so "CT-" and "CT" give "CT-123" and "CT123" respectively — the
// separator is the user's business, not ours to guess.
func numberLabelText(prefix string, number int, suffix string) string {
	return fmt.Sprintf("%s%d%s", prefix, number, suffix)
}

// fitNumberFont returns the largest size at or below numberFontMax at which the
// text fits between the margins. Without this a prefix simply ran off the label
// and was clipped, which you would not discover until the sheet came out of the
// printer.
func fitNumberFont(pdf *fpdfDoc, text string) float64 {
	size := numberFontMax
	for size > numberFontMin {
		pdf.SetFont("Arial", "B", size)
		if pdf.GetStringWidth(text) <= printableWidth {
			return size
		}
		size--
	}
	return numberFontMin
}

// numberLabelFits reports whether the text can be printed inside the printable
// width without going below numberFontMin. A long enough prefix cannot be made
// to fit at any readable size, and the honest answer then is to refuse it
// rather than print something clipped or too small to read across the yard.
func numberLabelFits(text string) bool {
	pdf := newLabelPDF()
	size := fitNumberFont(pdf, text)
	pdf.SetFont("Arial", "B", size)
	return pdf.GetStringWidth(text) <= printableWidth
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
