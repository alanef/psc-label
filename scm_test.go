package main

import (
	"strings"
	"testing"
	"time"
)

// historicMooringRow builds a row in the 2017 column layout, which is what the
// fallback positions encode.
func historicMooringRow(number, area, boatID, start, end string) []string {
	row := make([]string, 18)
	row[1] = number
	row[3] = area
	row[15] = boatID
	row[16] = start
	row[17] = end
	return row
}

func historicBoatRow(id, boat, contact, owner string) []string {
	row := make([]string, 45)
	row[0] = id
	row[1] = boat
	row[33] = contact
	row[44] = owner
	return row
}

func historicHeader(n int) []string {
	h := make([]string, n)
	for i := range h {
		h[i] = "col" + string(rune('A'+i%26))
	}
	return h
}

var testNow = time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)

func TestBuildLabelsHappyPath(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "01/Jan/2026", "31/Dec/2026"),
		historicMooringRow("12", "A", "B2", "01/Jan/2026", "31/Dec/2026"),
	}
	boats := [][]string{
		historicHeader(45),
		historicBoatRow("B1", "Solo:1246", "", "Zoe Adams"),
		historicBoatRow("B2", "Laser:9", "", "Al Baker"),
	}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 2 {
		t.Fatalf("got %d labels, want 2", len(res.Labels))
	}
	// Sorted by surname: Adams before Baker.
	if res.Labels[0].Name != "Zoe Adams" {
		t.Errorf("labels not sorted by surname: first is %q", res.Labels[0].Name)
	}
	if res.Labels[0].Berth != "C147" {
		t.Errorf("berth = %q, want C147 (area then number)", res.Labels[0].Berth)
	}
	if res.Labels[0].Boat != "Solo:1246" {
		t.Errorf("boat = %q, want Solo:1246", res.Labels[0].Boat)
	}
	if res.Labels[0].Year != 2026 {
		t.Errorf("year = %d, want 2026", res.Labels[0].Year)
	}
}

// A blank end date used to reach time.Parse and call log.Fatal, killing the
// whole server. It now means an open-ended licence.
func TestBuildLabelsBlankEndDateIsOpenEnded(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "01/Jan/2026", ""),
	}
	boats := [][]string{historicHeader(45), historicBoatRow("B1", "Solo:1246", "", "Zoe Adams")}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 1 {
		t.Fatalf("got %d labels, want 1 — an open-ended licence should still print", len(res.Labels))
	}
}

// An unparseable date used to be fatal too. Now the row is skipped and the
// remaining rows still print.
func TestBuildLabelsBadDateSkipsRowAndWarns(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "not a date", "31/Dec/2026"),
		historicMooringRow("12", "A", "B2", "01/Jan/2026", "31/Dec/2026"),
	}
	boats := [][]string{
		historicHeader(45),
		historicBoatRow("B1", "Solo:1246", "", "Zoe Adams"),
		historicBoatRow("B2", "Laser:9", "", "Al Baker"),
	}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 1 {
		t.Fatalf("got %d labels, want 1 (the good row)", len(res.Labels))
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(res.Warnings))
	}
	if !strings.Contains(res.Warnings[0], "Line 2") {
		t.Errorf("warning should name the offending line, got %q", res.Warnings[0])
	}
}

func TestBuildLabelsSkipsExpiredAndFutureLicences(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("1", "A", "B1", "01/Jan/2020", "31/Dec/2020"), // expired
		historicMooringRow("2", "A", "B1", "01/Jan/2030", "31/Dec/2030"), // future
		historicMooringRow("3", "A", "B1", "01/Jan/2026", "31/Dec/2026"), // current
	}
	boats := [][]string{historicHeader(45), historicBoatRow("B1", "Solo:1", "", "Zoe Adams")}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(res.Labels))
	}
	if res.Skipped != 2 {
		t.Errorf("skipped = %d, want 2", res.Skipped)
	}
}

// A licence ending today should still be printable — the original excluded it,
// because it compared against midnight.
func TestBuildLabelsIncludesFinalDayOfLicence(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("1", "A", "B1", "01/Jan/2026", "01/Jun/2026"),
	}
	boats := [][]string{historicHeader(45), historicBoatRow("B1", "Solo:1", "", "Zoe Adams")}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 1 {
		t.Errorf("a licence ending today should print, got %d labels", len(res.Labels))
	}
}

func TestBuildLabelsShortRowsDoNotPanic(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		{"only", "three", "columns"},
		historicMooringRow("12", "A", "B1", "01/Jan/2026", "31/Dec/2026"),
	}
	boats := [][]string{
		historicHeader(45),
		{"B1"}, // truncated boat row
		historicBoatRow("B1", "Solo:1", "", "Zoe Adams"),
	}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(res.Labels))
	}
}

func TestBuildLabelsUnknownBoatStillPrints(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "MISSING", "01/Jan/2026", "31/Dec/2026"),
	}
	boats := [][]string{historicHeader(45), historicBoatRow("B1", "Solo:1", "", "Zoe Adams")}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 1 {
		t.Fatalf("got %d labels, want 1 — an unmatched boat should still get a berth label", len(res.Labels))
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning naming the unmatched boat")
	}
}

func TestBuildLabelsEmptyFiles(t *testing.T) {
	if _, err := buildLabels([][]string{}, [][]string{historicHeader(45)}, testNow); err == nil {
		t.Error("expected an error for an empty mooring file")
	}
	if _, err := buildLabels([][]string{historicHeader(18)}, [][]string{}, testNow); err == nil {
		t.Error("expected an error for an empty boats file")
	}
}

func TestOwnerFallsBackToContactColumn(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("1", "A", "B1", "01/Jan/2026", "31/Dec/2026"),
	}
	boats := [][]string{historicHeader(45), historicBoatRow("B1", "Solo:1", "Pat Contact", "")}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Labels[0].Name != "Pat Contact" {
		t.Errorf("name = %q, want the fallback contact column", res.Labels[0].Name)
	}
}

// realBoatsHeader is the header row of the SCM boats export of 16 Aug 2026,
// verbatim. Header names only — the file itself holds members' details and must
// never be committed.
var realBoatsHeader = []string{
	"Boat ID", "Name", "Manufacturer", "Keel", "Construction", "Colour",
	"Nat Registration", "Insurance", "Disc Number", "LOA", "Beam", "Draught",
	"Vessel Type", "Usage", "Status", "Notes", "LOA Units", "Beam Units",
	"Draught Units", "LOA Feet", "LOA Inches", "Beam Feet", "Beam Inches",
	"Draught Feet", "Draught Inches", "Weight", "Mast Height", "Rig Type",
	"Design", "Mast Units", "Mast Feet", "Mast Inches", "Sail Number",
	"Owner Name", "Primary Name", "Primary Number", "Secondary Numbers",
	"IRC TCC", "IRC Crew Number", "IRC Cert Number", "Class", "Import ID",
	"Contact IDs", "Portsmouth Number", "Contact Name", "Contact Number",
	"Contact Email", "Location", "Tags", "Berth/Space Name",
	"Berth/Space Group", "Start", "End",
}

// realMooringHeader is the header row of the SCM mooring allocations export of
// 16 Aug 2026, verbatim. Note it no longer matches the 2017 positions: SCM
// inserted "Allocation Contact ID" ahead of the dates, which is what broke
// bulk printing.
var realMooringHeader = []string{
	"ID", "Name", "Type", "Group", "Note", "Latitude", "Longitude",
	"Max Length (m)", "Max Width (m)", "Max Draught (m)", "Max Height (m)",
	"Max Weight (kg)", "Allocation ID", "Allocation Boat", "Allocation Design",
	"Allocation Contact", "Allocation Contact ID", "Allocation Boat ID",
	"Allocation From", "Allocation Until", "Allocation Price",
	"Allocation Price Name", "Allocation Invoice", "Allocation Outstanding",
	"Allocation Disc Nunber", "Allocation Status", "Allocation Status Set At",
}

func TestResolveColumnsAgainstRealMooringExport(t *testing.T) {
	cols := resolveColumns(realMooringHeader, mooringColumns)

	want := map[string]int{
		"berthnumber":       1,  // Name
		"bertharea":         3,  // Group
		"boatid":            17, // Allocation Boat ID
		"start":             18, // Allocation From
		"end":               19, // Allocation Until
		"allocationboat":    13,
		"allocationcontact": 15,
	}
	for key, pos := range want {
		if got := cols.index[key]; got != pos {
			t.Errorf("%s resolved to column %d (%q), want %d (%q)",
				key, got, realMooringHeader[got], pos, realMooringHeader[pos])
		}
		if !cols.matchedByName(key) {
			t.Errorf("%s should match by header name, not fall back to a position", key)
		}
	}

	// The historic positions must NOT be used against this file: column 16 is
	// a contact ID, and feeding it to the date parser is the original bug.
	if cols.index["start"] == 16 {
		t.Error("start date resolved to the 2017 position, which is now Allocation Contact ID")
	}
}

// A boat that is in the moorings file but incomplete in the boats file should
// still get a full label — three of the club's 406 allocations are like this.
func TestGapsInBoatsFileAreFilledFromMooringRow(t *testing.T) {
	moorings := [][]string{realMooringHeader, make([]string, len(realMooringHeader))}
	row := moorings[1]
	row[1], row[3] = "530", "C"
	row[13], row[15] = "Container 146", "Craig Adams"
	row[17], row[18], row[19] = "407587", "01/Jan/2026", "31/Dec/2026"

	// The boat exists but has no name and no contact of any kind.
	boats := [][]string{realBoatsHeader, make([]string, len(realBoatsHeader))}
	boats[1][0] = "407587"

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(res.Labels))
	}
	if res.Labels[0].Name != "Craig Adams" || res.Labels[0].Boat != "Container 146" {
		t.Errorf("label = %+v, want the boat and contact from the mooring row", res.Labels[0])
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "fixing in SCM") {
		t.Errorf("expected a warning pointing at the SCM record, got %v", res.Warnings)
	}
}

// realMooringRow builds a row in the 2026 layout.
func realMooringRow(name, group, boat, contact, boatID, from, until string) []string {
	row := make([]string, len(realMooringHeader))
	row[1], row[3] = name, group
	row[13], row[15] = boat, contact
	row[17], row[18], row[19] = boatID, from, until
	return row
}

// The mooring file alone is enough: this is the one-download path.
func TestBuildLabelsFromMooringFileAlone(t *testing.T) {
	moorings := [][]string{
		realMooringHeader,
		realMooringRow("530", "C", "Solo:1246", "Zoe Adams", "407587", "01/Jan/2026", "31/Dec/2026"),
		realMooringRow("12", "A", "Laser:9", "Al Baker", "446917", "01/Jan/2026", "31/Dec/2026"),
	}

	res, err := buildLabels(moorings, nil, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Labels) != 2 {
		t.Fatalf("got %d labels, want 2", len(res.Labels))
	}
	if res.Labels[0].Name != "Zoe Adams" || res.Labels[0].Boat != "Solo:1246" || res.Labels[0].Berth != "C530" {
		t.Errorf("label = %+v, want the mooring row's own boat and contact", res.Labels[0])
	}
	if len(res.Warnings) != 0 {
		t.Errorf("a complete mooring row should warn about nothing, got %v", res.Warnings)
	}
}

// The boats file remains authoritative when it is supplied.
func TestBoatsFileWinsOverMooringRowWhenBothPresent(t *testing.T) {
	moorings := [][]string{
		realMooringHeader,
		realMooringRow("530", "C", "Stale Boat", "Stale Name", "407587", "01/Jan/2026", "31/Dec/2026"),
	}
	boats := [][]string{realBoatsHeader, make([]string, len(realBoatsHeader))}
	boats[1][0], boats[1][1], boats[1][44] = "407587", "Solo:1246", "Zoe Adams"

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Labels[0].Name != "Zoe Adams" || res.Labels[0].Boat != "Solo:1246" {
		t.Errorf("label = %+v, want the boats file values", res.Labels[0])
	}
}

// Without a boats file, an old-layout mooring export cannot be used on its own:
// its boat and contact columns cannot be identified, so say so plainly.
func TestMooringFileAloneRefusedWhenColumnsUnidentified(t *testing.T) {
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "01/Jan/2026", "31/Dec/2026"),
	}

	_, err := buildLabels(moorings, nil, testNow)
	if err == nil {
		t.Fatal("expected an error asking for the boats file")
	}
	if !strings.Contains(err.Error(), "boats file is needed") {
		t.Errorf("error should tell the user to upload the boats file, got %q", err)
	}
}

// Those same columns must NOT be read from a file whose layout was guessed by
// position — at 13 and 15 in the old layout they mean something else.
func TestMooringFallbackColumnsIgnoredWhenLayoutIsGuessed(t *testing.T) {
	moorings := [][]string{
		historicHeader(18), // meaningless headers, so everything falls back
		historicMooringRow("147", "C", "MISSING", "01/Jan/2026", "31/Dec/2026"),
	}
	boats := [][]string{historicHeader(45), historicBoatRow("B1", "Solo:1", "", "Zoe Adams")}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Labels[0].Name != "" || res.Labels[0].Boat != "" {
		t.Errorf("label = %+v, want blanks rather than data read from unidentified columns", res.Labels[0])
	}
}

// Pin the real export layout so a change to the alias lists cannot silently
// re-point a column.
func TestResolveColumnsAgainstRealBoatsExport(t *testing.T) {
	cols := resolveColumns(realBoatsHeader, boatColumns)

	want := map[string]int{
		"id":          0,  // Boat ID
		"boat":        1,  // Name
		"contactname": 44, // Contact Name
		"ownername":   33, // Owner Name
	}
	for key, pos := range want {
		if got := cols.index[key]; got != pos {
			t.Errorf("%s resolved to column %d (%q), want %d (%q)",
				key, got, realBoatsHeader[got], pos, realBoatsHeader[pos])
		}
	}
	for _, note := range cols.notes {
		if strings.Contains(note, "no matching header") {
			t.Errorf("every column should match by name in the real export: %s", note)
		}
	}
}

// The club's labels have always carried the Contact Name where there is one.
// Contact Name is populated on 78% of boats against 46% for Owner Name, and
// the two differ on 133 of them, so the order is not cosmetic.
func TestContactNameIsPreferredOverOwnerName(t *testing.T) {
	boats := [][]string{realBoatsHeader, make([]string, len(realBoatsHeader))}
	boats[1][0] = "B1"
	boats[1][1] = "Solo:1246"
	boats[1][33] = "Owner Person"
	boats[1][44] = "Contact Person"

	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "01/Jan/2026", "31/Dec/2026"),
	}

	res, err := buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Labels[0].Name != "Contact Person" {
		t.Errorf("name = %q, want the Contact Name column", res.Labels[0].Name)
	}

	// With no contact name, fall back to the owner.
	boats[1][44] = ""
	res, err = buildLabels(moorings, boats, testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Labels[0].Name != "Owner Person" {
		t.Errorf("name = %q, want the Owner Name fallback", res.Labels[0].Name)
	}
}

func TestResolveColumnsByHeaderName(t *testing.T) {
	// Columns in a completely different order from the historic layout.
	header := []string{"Boat ID", "Start Date", "End Date", "Berth No.", "Area"}
	cols := resolveColumns(header, mooringColumns)

	want := map[string]int{"boatid": 0, "start": 1, "end": 2, "berthnumber": 3, "bertharea": 4}
	for key, pos := range want {
		if got := cols.index[key]; got != pos {
			t.Errorf("%s resolved to column %d, want %d", key, got, pos)
		}
	}
}

func TestResolveColumnsFallsBackWhenHeadersUnknown(t *testing.T) {
	header := historicHeader(18) // meaningless names
	cols := resolveColumns(header, mooringColumns)

	if cols.index["start"] != 16 || cols.index["end"] != 17 {
		t.Errorf("expected fallback to historic positions, got start=%d end=%d", cols.index["start"], cols.index["end"])
	}
	for _, note := range cols.notes {
		if strings.Contains(note, "start:") && !strings.Contains(note, "historic column") {
			t.Errorf("fallback should be reported to the user, got %q", note)
		}
	}
}

func TestNormaliseHeader(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Berth No.", "berthno"},
		{" berth_no ", "berthno"},
		{"BERTH NO", "berthno"},
		{"Start Date", "startdate"},
	} {
		if got := normaliseHeader(tc.in); got != tc.want {
			t.Errorf("normaliseHeader(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseDateFormats(t *testing.T) {
	for _, in := range []string{"01/Jan/2026", "1/Jan/2026", "01-Jan-2026", "2026-01-01", "01/01/2026"} {
		got, err := parseDate(in)
		if err != nil {
			t.Errorf("parseDate(%q) failed: %v", in, err)
			continue
		}
		if got.Year() != 2026 || got.Month() != time.January || got.Day() != 1 {
			t.Errorf("parseDate(%q) = %v, want 2026-01-01", in, got)
		}
	}

	if got, err := parseDate("  "); err != nil || !got.IsZero() {
		t.Errorf("blank date should be zero and not an error, got %v / %v", got, err)
	}
	if _, err := parseDate("tuesday"); err == nil {
		t.Error("expected an error for an unparseable date")
	}
}

func TestSortKey(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Jo Bloggs", "Bloggs Jo"},
		{"Mary Jane Watson", "Watson Mary Jane"},
		{"Cher", "Cher"},
		{"", ""},
		{"  Jo   Bloggs  ", "Bloggs Jo"},
	} {
		if got := sortKey(tc.in); got != tc.want {
			t.Errorf("sortKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
