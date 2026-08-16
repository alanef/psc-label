# Contributing

This is a small tool that one club actually depends on, once a year, on a
volunteer's laptop. That shapes everything below.

## Never commit real data

The SCM exports contain members' names, boats, berth numbers and invoice
references. Generated bulk PDFs contain the same thing, laid out neatly.

- `.gitignore` blocks `*.csv` and `*.pdf`. Do not add exceptions.
- Do not paste a real export into an issue, a pull request or a test fixture.
- Tests build synthetic rows instead — see `historicMooringRow` and
  `historicBoatRow` in `scm_test.go`, and the fake SCM in `scmfetch_test.go`.

This is not hypothetical. The repository's original history had to be discarded
because it contained a bulk PDF with 282 members' details.

## The failures the design exists to prevent

The program was written in 2017 as a Go learning exercise and hardened in 2026
after it started dying mid-print. Each rule below is the scar of a specific
failure, so please do not undo them.

- **No `log.Fatal` in a request path.** A single unparseable date in a 300-row
  CSV used to exit the process; on Windows the console window simply vanished,
  which read to users as "it crashes without an error". Handler errors go in the
  page's error box. `log.Fatal` is for startup only.
- **Nothing is written to disk.** PDFs used to be written to the working
  directory, which failed when it was read-only, or when a Windows PDF viewer
  still held the file open — and that failure was also fatal. PDFs are built in
  memory and served from `pdfStore` under unguessable URLs.
- **No package-level request state.** Errors and the current PDF path were once
  globals, so two people using a hosted copy saw each other's output.
- **Row-level problems warn, they do not fail.** A bad date skips that row and
  says so; everything else still prints. A blank label is obvious on the bench,
  a missing one is not noticed until someone has no licence on their boat.
- **The working directory is not served.** The original mounted
  `http.FileServer(http.Dir("./"))` at `/`.

`assertStillServing` in `handlers_test.go` exists to prove these stay fixed: the
tests assert the server is *still answering* after bad input. If you change error
handling, watch those.

## Column mapping is a contract with SCM

`mooringColumns` and `boatColumns` in `scm.go` hold the alias lists, matched on
header text after `normaliseHeader` (lowercase, letters and digits only). The
historic 2017 positions are fallbacks of last resort and **should not change** —
mooring 1/3/15/16/17 and boats 0/1/33/44.

The mooring export has already drifted once: in the 16 Aug 2026 file SCM had
inserted "Allocation Contact ID" ahead of the dates, so position 16 became a
contact ID. The old code fed that to `time.Parse` and called `log.Fatal` on the
result. That is the whole story of the crash.

When a real export shows the matching is wrong, **add the actual header text to
the alias list** rather than changing a fallback. Both real header rows are
pinned in `scm_test.go` (`realMooringHeader`, `realBoatsHeader`) so an alias edit
cannot silently re-point a column.

## Talking to SCM

`scmfetch.go` signs in and downloads the mooring export. Two rules:

- **Credentials never persist and never surface.** Not to disk, not to a log,
  not into the rendered page, not into a URL. A rejected login logs a fixed
  string rather than SCM's own error, which is free to quote the email back.
  Tests assert this across every failure mode; keep them passing.
- **Ask SCM for the seven columns a label needs, and no more.** Widening
  `scmExportFields` pulls more personal data into the process and needs a real
  reason. A test asserts the unwanted fields are never requested.

Tests run against a fake SCM built in `scmfetch_test.go`. Do not point a test at
the live system.

## Style

- Match the surrounding code: tabs, plain `net/http` and `html/template`, no
  frameworks. Adding a dependency needs a good reason.
- User-visible problems go through `Page.Singleerr` / `Bulkerr` / `Numbererr` /
  `Fetcherr` (red), `Page.Warnings` (amber, per-row problems) and `Page.Notes`
  (grey, column resolution) — not HTTP error codes. The form page is the UI.
- Label geometry lives in the constants at the top of `pdf.go`. Changing it
  means updating the printer guidance in the template too, and
  `TestLabelPageSize` will fail until you update the expected MediaBox.

## Before you open a pull request

```
go test ./...
go test -race ./...
go vet ./... && gofmt -l .
```

CI runs the same, plus a cross-compile and a container smoke test. For a change
anyone will see, run it by hand as well — `go run .` and use the form.

Explain in the commit message *why*, not just what. The commit log is the only
record of which failure a given piece of defensiveness is guarding against.

## Licence

By contributing you agree your work is licensed under the [MIT Licence](LICENSE)
that covers this project.
