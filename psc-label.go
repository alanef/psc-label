// Command psc-label produces berth licence labels for Papercourt Sailing Club
// in a 60mm x 40mm layout.
//
// It serves a small web UI. Run with no PORT set and it picks 9080 and opens a
// browser, which is how the club uses it on Windows; set PORT and it just
// listens, which is how it runs in a container.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/pkg/browser"
)

//go:embed templates/index.html
var indexHTML string

// version is set at build time by the release workflow.
var version = "dev"

const (
	// maxUploadBytes bounds a single bulk request. The real exports are a few
	// hundred KB; this is generous but stops a stray upload exhausting memory.
	maxUploadBytes = 32 << 20
	// maxNumberLabels bounds the number-label range so a typo like 1-999999
	// cannot generate a document large enough to take the server down.
	maxNumberLabels = 2000
)

// Page is the view model for the single page this program serves.
type Page struct {
	Year         int
	Singleerr    []string
	Bulkerr      []string
	Numbererr    []string
	Warnings     []string // non-fatal problems with a bulk run
	Notes        []string // how CSV columns were resolved
	Summary      string   // e.g. "282 labels generated"
	Labelfile    string   // URL of the generated PDF, empty if none
	Downloadfile string   // same PDF, as an attachment

	// Fetch-from-SCM panel. Filled in by render for every page.
	Fetcherr  []string
	ShowFetch bool   // the panel is offered at all
	AskLogin  bool   // no credentials in the environment, so ask on the page
	SCMHost   string // shown so it is obvious which system is about to be contacted
}

// server holds everything the handlers share. The original kept the current
// PDF path and the error lists in package-level variables, which meant two
// people using a hosted copy at once would see each other's output.
type server struct {
	tmpl  *template.Template
	pdfs  *pdfStore
	start time.Time
	scm   scmConfig
}

func newServer() (*server, error) {
	tmpl, err := template.New("index").Parse(indexHTML)
	if err != nil {
		return nil, fmt.Errorf("parsing embedded template: %w", err)
	}
	return &server{tmpl: tmpl, pdfs: newPDFStore(8), start: time.Now(), scm: loadSCMConfig()}, nil
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/printsingle", s.handleSingle)
	mux.HandleFunc("/printbulk", s.handleBulk)
	mux.HandleFunc("/printnumber", s.handleNumber)
	mux.HandleFunc("/printfetch", s.handleFetch)
	mux.HandleFunc("/label/", s.handleLabelPDF)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	return mux
}

func main() {
	log.SetFlags(log.LstdFlags)

	port := os.Getenv("PORT")
	openBrowser := false
	if port == "" {
		port = "9080"
		openBrowser = true
	}

	// The container image has no shell, so Docker's HEALTHCHECK runs this
	// binary against itself.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheck(port))
	}

	srv, err := newServer()
	if err != nil {
		log.Fatalf("psc-label: %v", err)
	}

	// Cloudflare Access, checked here as well as at the edge. Startup is the
	// right place to be fatal: a misconfiguration must stop the program, not
	// quietly serve members' details to anyone who finds the origin.
	accessCfg, err := loadAccessConfig()
	if err != nil {
		log.Fatalf("psc-label: %v", err)
	}
	var handler http.Handler = srv.routes()
	if accessCfg.Enforced {
		handler = newAccessVerifier(accessCfg).middleware(handler)
		log.Printf(">>>> Cloudflare Access enforced for %s — requests without a valid token are refused", accessCfg.Issuer)
	} else {
		log.Println(">>>> Cloudflare Access is NOT enforced by this program. That is right for a laptop; if this copy is reachable from the internet, set CF_ACCESS_TEAM_DOMAIN and CF_ACCESS_AUD.")
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           recoverPanic(handler),
		ReadHeaderTimeout: 10 * time.Second,
		// Generous: a bulk run of a thousand labels takes a few seconds.
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	log.Printf(">>>> psc-label %s listening on http://localhost:%s", version, port)
	if openBrowser {
		log.Println(">>>> A browser should open now. If you see no form, hit refresh.")
		log.Println(">>>> Leave this window open while you are printing; close it when you are done.")
		go func() {
			// Never fatal: a headless or locked-down machine simply means the
			// user opens the URL themselves.
			if err := browser.OpenURL("http://localhost:" + port); err != nil {
				log.Printf(">>>> Could not open a browser automatically (%v) — open http://localhost:%s yourself.", err, port)
			}
		}()
	}

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("psc-label: %v", err)
	}
}

// healthcheck asks a already-running instance whether it is serving, and
// returns a process exit code.
func healthcheck(port string) int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

// recoverPanic keeps one bad request from killing the process. Nothing in the
// handlers should panic, but this program runs unattended on someone else's
// laptop once a year, so it gets a backstop.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic serving %s: %v", r.URL.Path, v)
				http.Error(w, "Something went wrong generating that. Go back and try again.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// render writes the page. A template failure is logged rather than fatal.
func (s *server) render(w http.ResponseWriter, p Page) {
	p.Year = time.Now().Year()
	p.ShowFetch = !s.scm.Disabled
	p.AskLogin = !s.scm.FromEnv
	p.SCMHost = s.scm.BaseURL

	// Render to a buffer first so a mid-render error cannot leave a half-built
	// page on the wire.
	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, p); err != nil {
		log.Printf("rendering page: %v", err)
		http.Error(w, "Could not render the page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("writing page: %v", err)
	}
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.render(w, Page{})
}

// handleLabelPDF serves a generated PDF out of memory.
func (s *server) handleLabelPDF(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/label/"), ".pdf")
	pdf, ok := s.pdfs.get(id)
	if !ok {
		http.Error(w, "That PDF has expired — generate it again.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("download") != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="psc-labels.pdf"`)
	}
	// ServeContent gives the browser's PDF viewer range request support.
	http.ServeContent(w, r, id+".pdf", s.start, bytes.NewReader(pdf))
}

func (s *server) handleSingle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, Page{Singleerr: []string{"Could not read the form: " + err.Error()}})
		return
	}

	name := strings.TrimSpace(r.FormValue("Name"))
	boat := strings.TrimSpace(r.FormValue("Boat"))
	berth := strings.TrimSpace(r.FormValue("Berth"))

	var errs []string
	if len(name) < 3 {
		errs = append(errs, "Name is too short.")
	}
	if len(boat) < 3 {
		errs = append(errs, "Boat is too short.")
	}
	if berth == "" {
		errs = append(errs, "Berth cannot be empty.")
	}
	if len(errs) > 0 {
		s.render(w, Page{Singleerr: errs})
		return
	}

	// Two copies: one for the boat, one for the trailer.
	pdf := newLabelPDF()
	year := time.Now().Year()
	addLabel(pdf, year, name, boat, berth)
	addLabel(pdf, year, name, boat, berth)

	s.finish(w, pdf, Page{}, func(p *Page, msg string) { p.Singleerr = []string{msg} }, "2 labels generated.")
}

func (s *server) handleNumber(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, Page{Numbererr: []string{"Could not read the form: " + err.Error()}})
		return
	}

	var errs []string
	// The original indexed r.Form["From"][0] directly, which panicked on a
	// request with no fields at all.
	from, err := strconv.Atoi(strings.TrimSpace(r.FormValue("From")))
	if err != nil {
		errs = append(errs, "Enter a whole number in FROM.")
	}
	to, err := strconv.Atoi(strings.TrimSpace(r.FormValue("To")))
	if err != nil {
		errs = append(errs, "Enter a whole number in TO.")
	}
	if len(errs) == 0 {
		switch {
		case from < 1:
			errs = append(errs, "FROM must be 1 or more.")
		case to < from:
			errs = append(errs, "TO cannot be less than FROM.")
		case to-from+1 > maxNumberLabels:
			errs = append(errs, fmt.Sprintf("That is %d labels — the most in one go is %d.", to-from+1, maxNumberLabels))
		}
	}
	if len(errs) > 0 {
		s.render(w, Page{Numbererr: errs})
		return
	}

	pdf := newLabelPDF()
	for i := from; i <= to; i++ {
		addNumberLabel(pdf, i)
	}

	s.finish(w, pdf, Page{}, func(p *Page, msg string) { p.Numbererr = []string{msg} },
		fmt.Sprintf("%d number labels generated (%d to %d).", to-from+1, from, to))
}

func (s *server) handleBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.render(w, Page{Bulkerr: []string{"Could not read the uploaded files: " + err.Error()}})
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	moorings, err := readCSV(r, "BerthFile")
	if err != nil {
		s.render(w, Page{Bulkerr: []string{"Mooring allocations file: " + err.Error()}})
		return
	}
	// The boats file is optional — the mooring file carries enough on its own.
	boats, err := readCSV(r, "BoatFile")
	if err != nil && !errors.Is(err, errNoFile) {
		s.render(w, Page{Bulkerr: []string{"Boats file: " + err.Error()}})
		return
	}

	s.printMoorings(w, moorings, boats, "that mooring file — check you downloaded the right one",
		func(p *Page, msg string) { p.Bulkerr = append(p.Bulkerr, msg) })
}

// printMoorings turns a mooring export (and optionally a boats export) into the
// label PDF. Shared by the upload form and the fetch-from-SCM button, so both
// routes report problems and warnings identically. source names the origin of
// the data for the "nothing current in it" message.
func (s *server) printMoorings(w http.ResponseWriter, moorings, boats [][]string, source string, setErr func(*Page, string)) {
	result, err := buildLabels(moorings, boats, time.Now())
	if err != nil {
		page := Page{Notes: result.Notes}
		setErr(&page, err.Error())
		s.render(w, page)
		return
	}
	if len(result.Labels) == 0 {
		page := Page{Notes: result.Notes, Warnings: result.Warnings}
		setErr(&page, fmt.Sprintf("No current licences found in %s — %d rows were outside their licence dates or unusable.", source, result.Skipped))
		s.render(w, page)
		return
	}

	// Two copies of each: one for the boat, one for the trailer.
	pdf := newLabelPDF()
	for _, l := range result.Labels {
		addLabel(pdf, l.Year, l.Name, l.Boat, l.Berth)
		addLabel(pdf, l.Year, l.Name, l.Boat, l.Berth)
	}

	page := Page{Notes: result.Notes, Warnings: result.Warnings}
	summary := fmt.Sprintf("%d labels for %d moorings (2 copies each); %d rows skipped as not current.",
		len(result.Labels)*2, len(result.Labels), result.Skipped)
	s.finish(w, pdf, page, setErr, summary)
}

// handleFetch signs in to SCM, downloads the mooring export and prints it, in
// one click. The upload form remains the fallback for when SCM changes.
func (s *server) handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	setErr := func(p *Page, msg string) { p.Fetcherr = append(p.Fetcherr, msg) }

	if s.scm.Disabled {
		page := Page{}
		setErr(&page, "Fetching from SCM is switched off on this copy.")
		s.render(w, page)
		return
	}

	if err := r.ParseForm(); err != nil {
		page := Page{}
		setErr(&page, "Could not read that form.")
		s.render(w, page)
		return
	}

	// Environment credentials win; otherwise take them from the form and let
	// them fall out of scope when this handler returns. Either way they are
	// never written down and never logged.
	creds := s.scm.Env
	if !s.scm.FromEnv {
		creds = scmCredentials{
			Email:    strings.TrimSpace(r.PostFormValue("scm_email")),
			Password: r.PostFormValue("scm_password"),
		}
	}
	if creds.Email == "" || creds.Password == "" {
		page := Page{}
		setErr(&page, "Enter the SCM email address and password to fetch from SCM.")
		s.render(w, page)
		return
	}

	client, err := newSCMClient(s.scm.BaseURL)
	if err != nil {
		page := Page{}
		setErr(&page, err.Error())
		s.render(w, page)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), scmFetchTimeout)
	defer cancel()

	moorings, err := client.fetchMoorings(ctx, creds)
	if err != nil {
		// Logged without the credentials, and without the error's own text for
		// a rejected login, which is the only case SCM could echo an email back.
		log.Printf("scm fetch failed: %s", logSafeFetchError(err))
		page := Page{}
		setErr(&page, err.Error())
		s.render(w, page)
		return
	}

	s.printMoorings(w, moorings, nil, "the mooring export fetched from SCM", setErr)
}

// logSafeFetchError keeps a rejected login out of the log entirely, in case a
// future SCM error page were to quote the email address back at us.
func logSafeFetchError(err error) string {
	if errors.Is(err, errSCMLogin) {
		return "SCM rejected the credentials"
	}
	return err.Error()
}

// finish renders the PDF, stores it and shows the page. setErr lets each
// handler report a failure against its own form.
func (s *server) finish(w http.ResponseWriter, pdf *fpdfDoc, page Page, setErr func(*Page, string), summary string) {
	data, err := renderPDF(pdf)
	if err != nil {
		// This used to be log.Fatal: the whole server exited and the user saw
		// nothing but a closed window.
		log.Printf("generating pdf: %v", err)
		setErr(&page, "Could not generate the PDF: "+err.Error())
		s.render(w, page)
		return
	}

	id := s.pdfs.put(data)
	page.Labelfile = "/label/" + id + ".pdf"
	page.Downloadfile = page.Labelfile + "?download=1"
	page.Summary = summary
	s.render(w, page)
}

// errNoFile means the form field was left empty, which is only an error for
// the mooring file.
var errNoFile = errors.New("no file was chosen")

// readCSV pulls one uploaded file out of the request and parses it leniently:
// ragged rows and stray quotes are tolerated, because these files are often
// opened and re-saved in Excel before being uploaded.
func readCSV(r *http.Request, field string) ([][]string, error) {
	file, header, err := r.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, errNoFile
		}
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%q is empty", header.Filename)
	}
	return parseCSV(data, header.Filename)
}

// parseCSV reads CSV leniently: ragged rows and stray quotes are tolerated,
// because these files are often opened and re-saved in Excel before being
// uploaded. name is only used to describe the source in an error.
func parseCSV(data []byte, name string) ([][]string, error) {
	// Excel writes a UTF-8 byte order mark, which would otherwise become part
	// of the first header name and break column matching.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1 // rows may be ragged
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("could not read %q as CSV: %w", name, err)
	}
	return rows, nil
}

// pdfStore keeps recently generated PDFs in memory. Bounded so a long session
// cannot grow without limit; the URLs are unguessable so a hosted copy does not
// leak one user's labels to another.
type pdfStore struct {
	mu    sync.Mutex
	items map[string][]byte
	order []string
	max   int
}

func newPDFStore(max int) *pdfStore {
	return &pdfStore{items: make(map[string][]byte, max), max: max}
}

func (s *pdfStore) put(data []byte) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := randomID()
	s.items[id] = data
	s.order = append(s.order, id)
	for len(s.order) > s.max {
		delete(s.items, s.order[0])
		s.order = s.order[1:]
	}
	return id
}

func (s *pdfStore) get(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.items[id]
	return data, ok
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice; if it ever does, a
		// time-based id is still fine for a single-page local tool.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
