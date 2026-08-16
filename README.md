# psc-label

Berth licence label printing for Papercourt Sailing Club.

A single self-contained Go executable. Run it, and it opens a browser on a small
local web page where you can produce PDFs of 60mm x 40mm labels — individually,
in bulk from the club's SCM exports, or as plain numbers.

## What it does

Three label types, all rendered to a PDF that is written next to the executable
and previewed in an iframe on the page (with a Print button):

| Form | Endpoint | Output file | Contents |
|------|----------|-------------|----------|
| Individual Label | `POST /printsingle` | `singlelabel.pdf` | One member's name, boat and berth — printed twice (boat + trailer) |
| Bulk Labels | `POST /printbulk` | `bulklabels.pdf` | Every current mooring allocation, sorted by surname, each printed twice |
| Number Labels | `POST /printnumber` | `numberlabel.pdf` | A range of large numbers, one per label |

Each label is a page of its own at 63mm x 43mm — the 60x40 print area plus the
non-printing margin — with a black `PSC Licence <year>` banner and the
name (truncated to 16 characters), boat and berth beneath it.

## Running it

Run the built executable (see [Building](#building)). By default it listens on port **9080** and launches
your browser at `http://localhost:9080`. If the `PORT` environment variable is
set it uses that instead and does *not* open a browser — that is the mode the
`Procfile` uses for hosted deployment.

The PDF is written into the current working directory and served from there, so
run the program somewhere writable.

## Bulk labels

Bulk printing needs two CSV exports from SCM (ClubMin), which the page links to
directly:

- **Mooring Allocations** — `https://papercourtsc.clubmin.net/moorings/export_setup`
- **Boats** — `https://papercourtsc.clubmin.net/boats/export`

The mooring file drives the output. Rows whose licence period does not span
today (start/end dates in `dd/Mon/yyyy` form) are skipped; each remaining row is
matched to the boats file by boat ID to pick up the boat description and the
owner's name.

You may delete unwanted rows from the mooring CSV, or re-sort it in a
spreadsheet, before uploading — but keep the header row and do not remove any
columns, as the code reads fixed column positions.

## Printing

The labels are designed for a 60mm wide x 40mm high print area with a 1.3mm
non-printing margin. You will usually need to create a user-defined page size in
Printer > Preferences, select it at print time, and choose "fit to page". Print
a short test range before committing to a bulk run.

## Building

There is no `go.mod`. Dependencies (`gofpdf`, `open-golang`) are vendored under
`vendor/` and pinned in `Godeps/Godeps.json`, GOPATH-style, so the source has to
sit inside `$GOPATH/src` at its import path and be built in GOPATH mode — a
plain `go build psc-label.go` from a checkout anywhere else fails to find the
vendored packages.

```
mkdir -p $GOPATH/src/gitlab.com/alan8
ln -s /path/to/this/checkout $GOPATH/src/gitlab.com/alan8/psc-label
GO111MODULE=off go build -o psc-label gitlab.com/alan8/psc-label
```

Cross-compiling for Windows:

```
GO111MODULE=off GOOS=windows GOARCH=amd64 go build -o psc-label.exe gitlab.com/alan8/psc-label
```

Verified with Go 1.22.
