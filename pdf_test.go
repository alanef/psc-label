package main

import (
	"bytes"
	"strings"
	"testing"
)

// countPages counts page objects in an uncompressed-structure PDF. fpdf emits
// one "/Type /Page" per page plus a single "/Type /Pages" catalogue.
func countPages(t *testing.T, data []byte) int {
	t.Helper()
	total := bytes.Count(data, []byte("/Type /Page"))
	catalogues := bytes.Count(data, []byte("/Type /Pages"))
	return total - catalogues
}

func TestRenderPDFProducesAPDF(t *testing.T) {
	pdf := newLabelPDF()
	addLabel(pdf, 2026, "Jo Bloggs", "Solo:1246", "C147")

	data, err := renderPDF(pdf)
	if err != nil {
		t.Fatalf("renderPDF: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("output does not start with %%PDF: %q", data[:min(8, len(data))])
	}
	if got := countPages(t, data); got != 1 {
		t.Errorf("page count = %d, want 1", got)
	}
}

func TestEachLabelIsItsOwnPage(t *testing.T) {
	pdf := newLabelPDF()
	for i := 0; i < 5; i++ {
		addLabel(pdf, 2026, "Jo Bloggs", "Solo:1246", "C147")
	}
	data, err := renderPDF(pdf)
	if err != nil {
		t.Fatalf("renderPDF: %v", err)
	}
	if got := countPages(t, data); got != 5 {
		t.Errorf("page count = %d, want 5", got)
	}
}

func TestNumberLabelsRender(t *testing.T) {
	pdf := newLabelPDF()
	for i := 1; i <= 3; i++ {
		addNumberLabel(pdf, "", i, "")
	}
	data, err := renderPDF(pdf)
	if err != nil {
		t.Fatalf("renderPDF: %v", err)
	}
	if got := countPages(t, data); got != 3 {
		t.Errorf("page count = %d, want 3", got)
	}
}

// The label is 63x43mm, not A4. A regression here means the club's printer
// setup stops matching the output.
func TestLabelPageSize(t *testing.T) {
	pdf := newLabelPDF()
	addLabel(pdf, 2026, "Jo Bloggs", "Solo:1246", "C147")
	data, err := renderPDF(pdf)
	if err != nil {
		t.Fatalf("renderPDF: %v", err)
	}
	// 63mm x 43mm expressed in points, as fpdf writes MediaBox.
	want := "/MediaBox [0 0 178.58 121.89]"
	if !strings.Contains(string(data), want) {
		t.Errorf("expected MediaBox %q in output", want)
	}
}

func TestLongNamesAreTruncatedNotWrapped(t *testing.T) {
	// Truncation keeps the name on one line so the boat and berth still fit.
	long := "Bartholomew Fotheringay-Smythe"
	if got := truncate(long, maxNameRunes); len([]rune(got)) != maxNameRunes {
		t.Errorf("truncate gave %d runes, want %d", len([]rune(got)), maxNameRunes)
	}
	// Accented names must not be cut mid-rune.
	if got := truncate("Zoë Ångström-Lindqvist", 5); got != "Zoë Å" {
		t.Errorf("truncate mangled multi-byte runes: %q", got)
	}
	// Short names are left alone.
	if got := truncate("Jo", maxNameRunes); got != "Jo" {
		t.Errorf("truncate(%q) = %q", "Jo", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// A prefix or suffix must never push the text past the printable width. It
// would be clipped in the PDF and not noticed until the sheet was printed.
func TestNumberLabelsWithAffixesStayOnTheLabel(t *testing.T) {
	cases := []struct{ prefix, suffix string }{
		{"", ""},
		{"CT-", ""},
		{"", "-XX"},
		{"CT-", "-XX"},
		{"BERTH-", "/26"},
	}
	for _, tc := range cases {
		name := tc.prefix + "N" + tc.suffix
		t.Run(name, func(t *testing.T) {
			pdf := newLabelPDF()
			// 9999 is the widest number the form allows.
			for _, n := range []int{1, 9999} {
				addNumberLabel(pdf, tc.prefix, n, tc.suffix)
				text := numberLabelText(tc.prefix, n, tc.suffix)
				size := fitNumberFont(pdf, text)
				pdf.SetFont("Arial", "B", size)
				if w := pdf.GetStringWidth(text); w > printableWidth {
					t.Errorf("%q at %.0fpt is %.1fmm wide, wider than the %.1fmm printable area",
						text, size, w, printableWidth)
				}
				if size > numberFontMax || size < numberFontMin {
					t.Errorf("font size %.0f outside the allowed %v-%v", size, numberFontMin, numberFontMax)
				}
				if !numberLabelFits(text) {
					t.Errorf("%q reported as not fitting, but it should", text)
				}
			}
			if _, err := renderPDF(pdf); err != nil {
				t.Fatalf("renderPDF: %v", err)
			}
		})
	}
}

// Some affixes cannot be made to fit at any readable size. Those must be
// reported as not fitting, so the handler can refuse them rather than print a
// clipped label — which would only be discovered on the printed sheet.
func TestOverlongAffixesAreReportedAsNotFitting(t *testing.T) {
	for _, text := range []string{
		numberLabelText("ABCDEFGHIJ", 9999, "ABCDEFGHIJ"),
		numberLabelText("VERYLONGPR", 1, "EFIXHERE00"),
	} {
		if numberLabelFits(text) {
			t.Errorf("%q reported as fitting a 55mm label, which it cannot", text)
		}
	}
}

// A bare number must still print at full size — the affix support must not
// shrink the ordinary case.
func TestPlainNumbersKeepTheFullSizeFont(t *testing.T) {
	pdf := newLabelPDF()
	addNumberLabel(pdf, "", 9999, "")
	if got := fitNumberFont(pdf, "9999"); got != numberFontMax {
		t.Errorf("font for 9999 = %.0f, want the full %.0f", got, numberFontMax)
	}
}

func TestNumberLabelText(t *testing.T) {
	for _, tc := range []struct {
		prefix, suffix string
		number         int
		want           string
	}{
		{"", "", 123, "123"},
		{"CT-", "", 123, "CT-123"},
		{"", "-XX", 123, "123-XX"},
		{"CT", "", 123, "CT123"},
	} {
		if got := numberLabelText(tc.prefix, tc.number, tc.suffix); got != tc.want {
			t.Errorf("numberLabelText(%q,%d,%q) = %q, want %q", tc.prefix, tc.number, tc.suffix, got, tc.want)
		}
	}
}
