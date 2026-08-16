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
		addNumberLabel(pdf, i)
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
