package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// The SCM (ClubMin) exports are the fragile part of this program: the club
// downloads them once a year and their column layout has changed before. The
// original version read hard-coded column positions and died on anything
// unexpected. We now look columns up by header name, fall back to the historic
// position when no header matches, and report what we did on the page so a
// wrong guess is visible rather than silent.

// columnSpec is one column we need out of an SCM export.
type columnSpec struct {
	key      string   // our internal name
	aliases  []string // acceptable header texts, already normalised
	fallback int      // column position used when no header matches
}

// Mooring allocations export. Verified against the SCM export of 16 Aug 2026,
// which has 27 columns headed "ID", "Name", "Type", "Group", ... "Allocation
// Boat ID", "Allocation From", "Allocation Until".
//
// This export has moved since 2017: SCM inserted "Allocation Contact ID"
// ahead of the dates, so the historic positions 15/16/17 now land on
// "Allocation Contact", "Allocation Contact ID" and "Allocation Boat ID". That
// is precisely why bulk printing broke — the old code fed a contact ID to the
// date parser. The historic positions stay as a last resort for an archived
// file, but the SCM names below are what should match.
var mooringColumns = []columnSpec{
	{"berthnumber", []string{"berthnumber", "berthno", "mooringnumber", "mooringno", "berthname", "mooringname", "spacename", "name", "berth", "number", "no"}, 1},
	{"bertharea", []string{"bertharea", "mooringarea", "berthgroup", "mooringgroup", "spacegroup", "group", "area", "section", "zone", "row"}, 3},
	{"boatid", []string{"allocationboatid", "boatid", "boatref", "boatreference", "boat"}, 15},
	{"start", []string{"allocationfrom", "startdate", "start", "fromdate", "from", "licencestart", "licensestart", "validfrom"}, 16},
	{"end", []string{"allocationuntil", "enddate", "end", "todate", "to", "until", "licenceend", "licenseend", "validto", "expiry", "expirydate", "expires"}, 17},
	// The mooring row carries its own copy of the boat and the member. These
	// are only used when the boat is missing from the boats file, and only
	// when they were matched by header name — see buildLabels.
	{"allocationboat", []string{"allocationboat"}, -1},
	{"allocationcontact", []string{"allocationcontact"}, -1},
}

// Boats export. Verified against the SCM export of 16 Aug 2026, which has 53
// columns headed "Boat ID", "Name", ... "Owner Name", ... "Contact Name".
var boatColumns = []columnSpec{
	{"id", []string{"boatid", "id", "boatnumber"}, 0},
	{"boat", []string{"name", "boat", "boatname", "classandsailnumber", "classsailno", "description"}, 1},
	// "Contact Name" is the member who actually holds the boat and is filled
	// in far more often than "Owner Name" (78% vs 46% of 1163 boats in the
	// 2026 export), and the two disagree on 133 of them. The original program
	// preferred it by position for the same reason, so keep that order: the
	// club is used to the names these labels carry.
	{"contactname", []string{"contactname", "contact", "primarycontact", "helm"}, 44},
	{"ownername", []string{"ownername", "owner", "membername", "member", "ownerfullname"}, 33},
}

// normaliseHeader reduces a header cell to letters and digits only, so
// "Berth No.", "berth_no" and "BERTH NO" all compare equal.
func normaliseHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// columnMap is the resolved position of each column we need.
type columnMap struct {
	index  map[string]int
	byName map[string]bool // true when matched by header name, not by position
	notes  []string        // human-readable account of how each column was resolved
}

// matchedByName reports whether a column was found by its header rather than
// guessed from its historic position. A column whose meaning depends on the
// export's layout is only safe to read when this is true.
func (c columnMap) matchedByName(key string) bool { return c.byName[key] }

// cell reads a column from a row, returning "" when the row is short rather
// than panicking — short rows are common in hand-edited exports.
func (c columnMap) cell(row []string, key string) string {
	i, ok := c.index[key]
	if !ok || i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// resolveColumns matches each spec against the header row, falling back to the
// historic column position when no header matches.
func resolveColumns(header []string, specs []columnSpec) columnMap {
	normalised := make([]string, len(header))
	for i, h := range header {
		normalised[i] = normaliseHeader(h)
	}

	cm := columnMap{
		index:  make(map[string]int, len(specs)),
		byName: make(map[string]bool, len(specs)),
	}
	used := make(map[int]string, len(specs))

	for _, spec := range specs {
		found := -1
		for _, alias := range spec.aliases {
			for i, h := range normalised {
				if h == alias {
					if owner, taken := used[i]; taken && owner != spec.key {
						continue // another column already claimed this one
					}
					found = i
					break
				}
			}
			if found >= 0 {
				break
			}
		}

		if found >= 0 {
			cm.index[spec.key] = found
			cm.byName[spec.key] = true
			used[found] = spec.key
			cm.notes = append(cm.notes, fmt.Sprintf("%s: column %d (%q)", spec.key, found+1, strings.TrimSpace(header[found])))
			continue
		}

		cm.index[spec.key] = spec.fallback
		if spec.fallback < 0 {
			continue // optional column, absent from this export
		}
		name := "beyond end of row"
		if spec.fallback < len(header) {
			name = fmt.Sprintf("%q", strings.TrimSpace(header[spec.fallback]))
		}
		cm.notes = append(cm.notes, fmt.Sprintf("%s: no matching header, using historic column %d (%s)", spec.key, spec.fallback+1, name))
	}
	return cm
}

// dateFormats are tried in order against the licence date columns. SCM has
// exported "02/Jan/2006" historically; the others are cheap insurance.
var dateFormats = []string{
	"02/Jan/2006",
	"2/Jan/2006",
	"02-Jan-2006",
	"2006-01-02",
	"02/01/2006",
	"2/1/2006",
	"02-01-2006",
	time.RFC3339,
	"2006-01-02 15:04:05",
}

// parseDate tries each known format. An empty value is not an error: it means
// "not set", which the caller interprets per column.
func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	for _, f := range dateFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q", s)
}

// Label is one licence label to be printed.
type Label struct {
	Year     int
	Name     string
	SortName string
	Boat     string
	Berth    string
}

// bySurname orders labels by the sort key built in buildLabels.
type bySurname []Label

func (a bySurname) Len() int           { return len(a) }
func (a bySurname) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a bySurname) Less(i, j int) bool { return a[i].SortName < a[j].SortName }

// sortKey puts the last word of a name first, so "Jo Bloggs" sorts under "B".
func sortKey(name string) string {
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return fmt.Sprintf("%s %s", parts[len(parts)-1], strings.Join(parts[:len(parts)-1], " "))
	}
}

// buildResult carries everything the page needs to report on a bulk run.
type buildResult struct {
	Labels   []Label
	Notes    []string // how columns were resolved
	Warnings []string // rows we could not use
	Skipped  int      // rows outside their licence period or unusable
}

// buildLabels merges the two SCM exports into a sorted list of labels.
//
// Every row-level problem is collected as a warning and the row skipped. This
// function must never be able to take the process down: a single bad date in a
// 300-row export used to kill the server outright.
func buildLabels(moorings, boats [][]string, now time.Time) (buildResult, error) {
	var res buildResult

	if len(moorings) < 2 {
		return res, fmt.Errorf("mooring allocations file has no data rows (expected a header row then one row per mooring)")
	}
	if len(boats) < 2 {
		return res, fmt.Errorf("boats file has no data rows (expected a header row then one row per boat)")
	}

	mcols := resolveColumns(moorings[0], mooringColumns)
	bcols := resolveColumns(boats[0], boatColumns)
	res.Notes = append(res.Notes, "Mooring file — "+strings.Join(mcols.notes, "; "))
	res.Notes = append(res.Notes, "Boats file — "+strings.Join(bcols.notes, "; "))

	index := indexBoats(boats, bcols)

	const maxWarnings = 25 // enough to diagnose, not enough to bury the page
	warn := func(format string, args ...interface{}) {
		if len(res.Warnings) < maxWarnings {
			res.Warnings = append(res.Warnings, fmt.Sprintf(format, args...))
		}
	}

	for i, row := range moorings[1:] {
		line := i + 2 // 1-based, and the header is line 1

		startRaw := mcols.cell(row, "start")
		if startRaw == "" {
			res.Skipped++
			continue // no licence start: nothing to print, and normal in these exports
		}

		start, err := parseDate(startRaw)
		if err != nil {
			res.Skipped++
			warn("Line %d: %v in the start date column — row skipped.", line, err)
			continue
		}

		end, err := parseDate(mcols.cell(row, "end"))
		if err != nil {
			res.Skipped++
			warn("Line %d: %v in the end date column — row skipped.", line, err)
			continue
		}
		// A blank end date means an open-ended licence rather than an error.
		// A dated one runs to the end of that day, so labels can still be
		// printed on the final day of the licence.
		if !end.IsZero() {
			end = end.AddDate(0, 0, 1).Add(-time.Nanosecond)
		}

		if now.Before(start) || (!end.IsZero() && now.After(end)) {
			res.Skipped++
			continue // licence not current — expected for most rows
		}

		boatID := mcols.cell(row, "boatid")
		boat, owner := index.lookup(boatID)

		// The mooring row carries its own copy of the boat and the member, so
		// a gap in the boats file need not mean a half-blank label. Only trust
		// those columns when they were matched by header name — at their
		// historic positions they mean something else entirely.
		filledFromMooring := false
		if boat == "" && mcols.matchedByName("allocationboat") {
			if boat = mcols.cell(row, "allocationboat"); boat != "" {
				filledFromMooring = true
			}
		}
		if owner == "" && mcols.matchedByName("allocationcontact") {
			if owner = mcols.cell(row, "allocationcontact"); owner != "" {
				filledFromMooring = true
			}
		}

		switch {
		case filledFromMooring:
			warn("Line %d: boat %q is incomplete in the boats file — filled in from the mooring row. Worth fixing in SCM.", line, boatID)
		case boat == "" && owner == "":
			warn("Line %d: boat %q is not in the boats file — label printed without a name.", line, boatID)
		}

		berth := mcols.cell(row, "bertharea") + mcols.cell(row, "berthnumber")
		if berth == "" {
			warn("Line %d: no berth found — label printed without one.", line)
		}

		res.Labels = append(res.Labels, Label{
			Year:     now.Year(),
			Name:     owner,
			SortName: sortKey(owner),
			Boat:     boat,
			Berth:    berth,
		})
	}

	sort.Stable(bySurname(res.Labels))
	return res, nil
}

// boatIndex maps boat ID to its description and owner.
type boatIndex map[string]struct{ boat, owner string }

func (b boatIndex) lookup(id string) (boat, owner string) {
	if e, ok := b[strings.TrimSpace(id)]; ok {
		return e.boat, e.owner
	}
	return "", ""
}

// indexBoats builds the lookup once, instead of rescanning the boats file for
// every mooring row as the original did.
func indexBoats(boats [][]string, cols columnMap) boatIndex {
	index := make(boatIndex, len(boats))
	for _, row := range boats[1:] {
		id := cols.cell(row, "id")
		if id == "" {
			continue
		}
		owner := cols.cell(row, "contactname")
		if owner == "" {
			owner = cols.cell(row, "ownername")
		}
		if _, exists := index[id]; exists {
			continue // first entry wins, matching the original's behaviour
		}
		index[id] = struct{ boat, owner string }{cols.cell(row, "boat"), owner}
	}
	return index
}
