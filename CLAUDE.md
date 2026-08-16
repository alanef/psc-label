# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

A small Go web app that produces PDF berth-licence labels for Papercourt Sailing
Club. See `README.md` for user-facing behaviour. It runs two ways from one
binary: double-clicked on a club volunteer's Windows laptop (opens a browser on
localhost), or in a container behind an authenticating proxy.

## Layout

| File | Contents |
|------|----------|
| `psc-label.go` | `main`, HTTP server, handlers, the in-memory `pdfStore` |
| `scm.go` | SCM CSV parsing: column resolution, date handling, merging the exports |
| `pdf.go` | Label geometry and rendering; the only file that imports the PDF library |
| `templates/index.html` | The entire UI (inlined Skeleton CSS), `//go:embed`ed |
| `Dockerfile` | Static build on `scratch`, ~8MB, non-root, self-health-checking |
| `.github/workflows/` | `ci.yml` (fmt, vet, race tests, cross-compile, container smoke test) and `release.yml` (tagged builds) |

Standard modules, Go 1.22+. No vendor directory, no Godeps.

## The context that matters

This was written in 2017 as a Go learning exercise and has been in club use ever
since. It was hardened in 2026 after it started dying mid-print. **Every design
decision below exists because of a specific failure**, so please do not undo
them:

- **No `log.Fatal` in request paths.** The original called a `check(err)` helper
  that was `log.Fatal`, from inside handlers. A single unparseable date in a
  300-row CSV exited the process; on Windows the console window just vanished,
  which read to users as "it crashes without an error". Handler errors go into
  the page's error box. `log.Fatal` is for startup only.
- **Nothing is written to disk.** PDFs used to be written to the working
  directory and served as static files, which failed when the directory was
  read-only, or when the browser's PDF viewer still held the file open on
  Windows — and that failure was also fatal. PDFs are now built in memory and
  served from `pdfStore` under unguessable URLs.
- **No package-level request state.** Errors and the current PDF path used to be
  globals, so two people using a hosted copy would see each other's output.
- **CSV columns are matched by header name** (`scm.go`), with the historic
  position as a fallback and a note on the page saying which was used. Fixed
  indices are what broke when SCM changed its export.
- **The working directory is not served.** The original had
  `http.FileServer(http.Dir("./"))` mounted at `/`.
- **Row-level problems are warnings, not failures.** A bad date or an unmatched
  boat skips that row and reports it; the rest of the run still prints.

## Column mapping is a contract with SCM

`mooringColumns` and `boatColumns` in `scm.go` hold the alias lists. The historic
2017 positions (mooring 1/3/15/16/17, boats 0/1/33/44) are the fallbacks and
should not be changed — they are the last resort when no header matches.

When a real export shows the matching is wrong, add the actual header text to
the relevant alias list rather than changing a fallback. Aliases are compared
after `normaliseHeader`, which lowercases and strips everything but letters and
digits.

## Conventions

- Match the surrounding style: tabs, plain `net/http` and `html/template`, no
  frameworks. Adding a dependency needs a good reason.
- User-visible problems go through `Page.Singleerr` / `Bulkerr` / `Numbererr`
  (red), `Page.Warnings` (amber, per-row problems) and `Page.Notes` (grey,
  column resolution). Not HTTP error codes — the form page is the UI.
- Label geometry lives in the constants at the top of `pdf.go`. Changing it
  means updating the printer guidance text in the template too, and
  `TestLabelPageSize` will fail until you update the expected MediaBox.

## Testing

```
go test ./...          # 38 tests, no network or fixtures needed
go test -race ./...
go vet ./... && gofmt -l .
```

`scm_test.go` covers CSV merging and column resolution, `pdf_test.go` covers
geometry and page counts, `handlers_test.go` drives a real `httptest` server.

Several tests exist specifically to prove old crashes stay fixed — they assert
the server is *still serving* after bad input (`assertStillServing`). If you
change error handling, those are the ones to watch.

There is no fixture of a real SCM export, because the real files contain members'
names, boats and berth numbers. Tests build synthetic rows in the historic
layout via `historicMooringRow` / `historicBoatRow`. **Never commit a real
export or a generated bulk PDF** — `.gitignore` blocks `*.csv` and `*.pdf`, and
the repository's original history had to be discarded because it contained a
bulk PDF with 282 members' details.

To check something by hand, `go run .` and use the form; the container path is
covered by `docker build . && docker run -p 8080:8080`.
