# psc-label

Berth licence label printing for Papercourt Sailing Club.

A single self-contained binary. Run it and it opens a browser on a small local
web page where you can produce PDFs of 60mm x 40mm labels — for every current
mooring at once, straight from SCM or from an uploaded export, individually, or
as plain numbers. The same binary runs in a container behind a URL, so it can be
used either way.

## What it does

Three label types. Each PDF is generated in memory and shown in the page with a
Print button; nothing is written to disk.

| Form | Endpoint | Contents |
|------|----------|----------|
| Fetch from SCM | `POST /printfetch` | Signs in to SCM, downloads the current mooring allocations and prints the lot, in one click |
| Individual Label | `POST /printsingle` | One member's name, boat and berth — printed twice, for the boat and the trailer |
| Upload the CSV | `POST /printbulk` | Every current mooring allocation from an uploaded export, sorted by surname, each printed twice |
| Number Labels | `POST /printnumber` | A range of large numbers, one per label (max 2000 at a time) |

Each label is its own page at 63mm x 43mm — the 60x40 print area plus the
non-printing margin — with a black `PSC Licence <year>` banner and the name
(truncated to 16 characters), boat and berth beneath it.

## Running it on Windows

Download `psc-label.exe` from the
[latest release](https://github.com/alanef/psc-label/releases/latest) and
double-click it. A console window opens and your browser opens the form.

Windows may warn *"Windows protected your PC"* — click **More info**, then **Run
anyway**. That is because the file is not code-signed, not because there is
anything wrong with it. Leave the console window open while printing; close it
when you are done.

By default it listens on port 9080 and opens `http://localhost:9080`. Set the
`PORT` environment variable and it uses that port and does not open a browser —
which is how it runs in a container.

## Fetching from SCM

The **Fetch from SCM** button does the whole job: it signs in to SCM (ClubMin),
downloads the current mooring allocations and generates the labels, without
anyone visiting the export screen. It takes around ten seconds.

If no credentials are configured the page asks for an SCM email address and
password. They are used for that one request and then discarded — never written
to disk, never logged, never rendered back into the page.

It asks SCM for **only the seven columns a label needs**: `moor_name`,
`moor_group`, `alloc_boat`, `alloc_contact`, `alloc_boat_id`, `alloc_from` and
`alloc_until`. Invoice numbers, prices, member IDs and berth coordinates — all
present in the full 27-column export — never enter the process at all. A test
asserts the unwanted fields are never requested.

### Configuration

All optional. Set them in Coolify (or the shell) for the hosted copy; the
double-clicked Windows binary normally needs none of them.

| Variable | Default | Meaning |
|----------|---------|---------|
| `PORT` | `9080`, and opens a browser | Port to listen on. Setting it also suppresses the browser, which is how the container runs. |
| `SCM_BASE_URL` | `https://papercourtsc.clubmin.net` | The SCM instance to sign in to. |
| `SCM_EMAIL` | *(unset)* | SCM login. When both this and `SCM_PASSWORD` are set, the page stops asking and uses these instead. |
| `SCM_PASSWORD` | *(unset)* | As above. Only takes effect when the email is set too — half a credential is ignored rather than sent. |
| `SCM_FETCH` | *(unset)* | Set to `off` to hide the fetch panel entirely and leave only the CSV upload. |

**Think before setting `SCM_EMAIL` and `SCM_PASSWORD` on the hosted copy.**
Anyone who can reach the page can then pull the mooring list without knowing an
SCM password, and anyone who can read the environment has the credential itself.
If SCM has no read-only or mooring-scoped account, prefer leaving them unset and
letting whoever is printing type their own login.

The flow is four requests — `GET /login`, `POST /users/login`,
`GET /moorings/export_setup`, `POST /moorings/export_actual` — with the Rails
CSRF token scraped from each form. It is pinned in
[`scmfetch_test.go`](scmfetch_test.go) against a fake SCM, so the tests never
touch the live system.

## Uploading the CSV instead

Folded away behind an accordion on the page, and the fallback for when SCM
changes and the fetch stops working. It needs one export, which the page links
to directly:

- **Mooring Allocations** — `/moorings/export_setup` on your SCM instance

That one file is enough. A **Boats** export (`/boats/export`) is still accepted
if posted — it is the better source of boat names and members — but nothing on
the page offers one any more, and without it the names come from the mooring
rows themselves.

Rows whose licence period does not cover today are skipped. A row with no end
date is treated as an open-ended licence, and a licence ending today still
prints.

You may delete unwanted rows from the mooring CSV, or re-sort it in a
spreadsheet, before uploading — but keep the header row. Columns can move: they
are matched by header name, falling back to their historic position when a
header is not recognised. After each run the page prints exactly which column it
used for each field, so a bad guess is visible rather than silent. If it guesses
wrong, add the real header name to the alias lists at the top of
[`scm.go`](scm.go).

Rows that cannot be used — an unreadable date, a boat with no name anywhere —
are skipped and reported at the top of the page. This applies to both routes:
one bad row never stops the rest of the run.

## Printing

The labels are designed for a 60mm wide x 40mm high print area with a 1.3mm
non-printing margin. You will usually need to create a user-defined page size in
Printer > Preferences, select it at print time, and choose "fit to page". Print
a short test range before committing to a bulk run.

## Running it hosted

The `Dockerfile` produces an ~8MB image from `scratch`: no shell, no package
manager, running as UID 65534. It needs no volume. Its only outbound connection
is to SCM, when someone presses **Fetch from SCM**; set `SCM_FETCH=off` and it
makes none.

```
docker build -t psc-label .
docker run -p 8080:8080 psc-label
```

In Coolify, point an application at this repository and let it build from the
Dockerfile. It listens on `$PORT` (8080 in the image) and answers `/healthz`.

**Put an authenticating proxy in front of it.** The app itself has no login, and
the CSVs contain members' names, boats and berth numbers. Cloudflare Zero Trust
with an email allowlist is the intended arrangement.

Uploaded and fetched CSVs and generated PDFs are held in memory only: the CSV is
discarded once the labels are built, and the last few PDFs are kept under
unguessable URLs until they are evicted or the process restarts. Nothing reaches
disk, and SCM credentials are never stored or logged.

## Development

```
go test ./...        # unit and HTTP handler tests
go vet ./...
go run .             # starts on :9080 and opens a browser
PORT=8080 go run .   # starts on :8080, no browser
```

Go 1.22 or newer. Dependencies are ordinary modules —
[`go-pdf/fpdf`](https://github.com/go-pdf/fpdf) for PDF generation and
`pkg/browser` for the launch.

| File | Contents |
|------|----------|
| `psc-label.go` | HTTP server, handlers, in-memory PDF store |
| `scm.go` | SCM CSV parsing: column matching, dates, merging the two exports |
| `scmfetch.go` | Signing in to SCM and downloading the mooring export |
| `pdf.go` | Label geometry and PDF rendering |
| `templates/index.html` | The whole UI, embedded into the binary at build time |

### Releases

Tagging builds and publishes the binaries, so the club never depends on anyone's
laptop for a working copy:

```
git tag v1.0.0 && git push origin v1.0.0
```

The workflow attaches `psc-label.exe` (Windows), Linux and macOS builds and a
`SHA256SUMS.txt` to a GitHub release.
