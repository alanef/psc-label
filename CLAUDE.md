# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

A single-file Go program (`psc-label.go`, ~1400 lines) that serves a local web
page for producing PDF berth-licence labels for Papercourt Sailing Club. See
`README.md` for user-facing behaviour.

## Layout

- `psc-label.go` — everything: the HTML/CSS UI, HTTP handlers, CSV parsing, PDF generation.
- `vendor/` — vendored `github.com/jung-kurt/gofpdf` and `github.com/skratchdot/open-golang/open`.
- `Godeps/Godeps.json` — dependency pins (godep, GOPATH era).
- `Procfile` — `web: psc-label`, for hosted deployment where `PORT` is set.

There is **no `go.mod`**. The project predates modules and relies on GOPATH
vendoring, so it only builds in GOPATH mode with the source at its import path:

```
mkdir -p $GOPATH/src/gitlab.com/alan8
ln -s /path/to/this/checkout $GOPATH/src/gitlab.com/alan8/psc-label
GO111MODULE=off go build -o psc-label gitlab.com/alan8/psc-label
```

`go build`/`go vet` run directly in the checkout fail ("cannot find package
github.com/jung-kurt/gofpdf", "directory prefix . does not contain main
module") — that is expected, not a broken tree. Do not add a `go.mod` or
re-vendor unless asked; it changes how the thing is built and deployed.

## Structure of psc-label.go

Roughly, in file order:

- Lines ~22–1067: `const testp` — the entire UI as one Go raw-string
  `html/template`. Normalize.css + Skeleton CSS is inlined in a `<style>` block,
  so most of the file is CSS, not logic. Template fields come from `Page`
  (`Singleerr`, `Bulkerr`, `Numbererr`, `Year`, `Labelfile`).
- `Page`, `Output`, `SurnameSorter` — view model, label record, surname sort.
- Package-level `singleError` / `bulkError` / `numberError` / `outputFile` —
  mutable globals holding state between the POST handler and the next GET of
  `/`. Handlers set them, redirect to `/`, and `handler` renders and clears
  them. This is single-user-by-design; it is not concurrency-safe and the last
  generated PDF is global. Keep that in mind before "improving" a handler.
- `main` — reads `PORT` (default `9080`, and only then opens a browser),
  registers the four routes, serves the CWD as static files via `chttp` for any
  path containing a dot (that is how the generated PDF is served back).
- `printsingle`, `printbulk`, `printnumber` — form handlers.
- `searchBoats`, `pdfOutput`, `pdfContent`, `pdfNumberContent`, `short`, `check`.

## CSV column positions

`printbulk` reads fixed column indices from the SCM exports. Changing these
silently produces wrong labels, so treat them as a contract with SCM:

Mooring allocations CSV (`berth`, header row skipped):
- `[1]`, `[3]` — berth number and area; the label berth is `berth[3] + berth[1]`
- `[15]` — boat ID, used to look up the boats CSV
- `[16]`, `[17]` — licence start/end, parsed as `02/Jan/2006`; rows not spanning today are skipped, and rows with an empty `[16]` are skipped

Boats CSV (`records`, header row skipped):
- `[0]` — boat ID (match key)
- `[1]` — boat description used on the label
- `[44]` — preferred owner name; falls back to `[33]` when empty

## Conventions to follow

- Match the existing style: tabs, plain `net/http` + `html/template`, no
  frameworks, no new dependencies.
- `check(err)` is `log.Fatal` on error — it kills the server. It is used freely
  in handlers (e.g. date parsing in `printbulk`), which means malformed input
  can take the process down. That is existing behaviour; only change it if the
  task is about robustness.
- User-visible errors go through the `*Error` slices and the red `.error` block
  in the template, not HTTP error codes.
- Label geometry lives in `pdfContent` / `pdfNumberContent`: page format
  63x43mm, margins `5, 3, 3`. Any layout change belongs there, and the printer
  guidance text in the template may need updating with it.

## Verifying changes

There are no tests. To check work, build as above and run the binary from a
writable directory (set `PORT` to suppress the browser launch), submit the
relevant form, and inspect the generated PDF —
`singlelabel.pdf`, `bulklabels.pdf` or `numberlabel.pdf` in the CWD. Bulk
changes need real SCM-shaped CSVs; do not assume a handler is correct without
producing a PDF.

Generated PDFs are build artefacts — do not commit them (earlier commits removed
`bulklabels.pdf` for this reason).
