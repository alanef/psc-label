package main

import (
	"bytes"
	"encoding/csv"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, err := newServer()
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	ts := httptest.NewServer(recoverPanic(s.routes()))
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, ts *httptest.Server, path string, form url.Values) (int, string) {
	t.Helper()
	resp, err := ts.Client().PostForm(ts.URL+path, form)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return resp.StatusCode, string(body)
}

var labelURLRe = regexp.MustCompile(`/label/[0-9a-f]+\.pdf`)

// fetchGeneratedPDF pulls the PDF the page is pointing at.
func fetchGeneratedPDF(t *testing.T, ts *httptest.Server, body string) []byte {
	t.Helper()
	match := labelURLRe.FindString(body)
	if match == "" {
		t.Fatalf("no generated PDF link in page")
	}
	resp, err := ts.Client().Get(ts.URL + match)
	if err != nil {
		t.Fatalf("fetching %s: %v", match, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetching PDF: status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading PDF: %v", err)
	}
	return data
}

// assertStillServing is the point of most of these tests: the original exited
// the process on bad input, so every failure case must be followed by proof
// that the server is still up.
func assertStillServing(t *testing.T, ts *httptest.Server) {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("server died: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("server unhealthy after bad request: status %d", resp.StatusCode)
	}
}

func TestIndexRenders(t *testing.T) {
	ts := newTestServer(t)
	resp, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	for _, want := range []string{"/printsingle", "/printbulk", "/printnumber", "PSC Berth Label Printing"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("page is missing %q", want)
		}
	}
}

func TestUnknownPathIs404(t *testing.T) {
	ts := newTestServer(t)
	resp, err := ts.Client().Get(ts.URL + "/etc/passwd")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	// The original served the whole working directory as static files.
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — the working directory must not be served", resp.StatusCode)
	}
}

func TestSingleLabelHappyPath(t *testing.T) {
	ts := newTestServer(t)
	status, body := post(t, ts, "/printsingle", url.Values{
		"Name":  {"Jo Bloggs"},
		"Boat":  {"Solo:1246"},
		"Berth": {"C147"},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data := fetchGeneratedPDF(t, ts, body)
	if got := countPages(t, data); got != 2 {
		t.Errorf("page count = %d, want 2 (one for the boat, one for the trailer)", got)
	}
}

func TestSingleLabelValidation(t *testing.T) {
	ts := newTestServer(t)
	for _, tc := range []struct {
		name string
		form url.Values
		want string
	}{
		{"short name", url.Values{"Name": {"Jo"}, "Boat": {"Solo:1"}, "Berth": {"C1"}}, "Name is too short"},
		{"short boat", url.Values{"Name": {"Jo Bloggs"}, "Boat": {"S"}, "Berth": {"C1"}}, "Boat is too short"},
		{"no berth", url.Values{"Name": {"Jo Bloggs"}, "Boat": {"Solo:1"}, "Berth": {""}}, "Berth cannot be empty"},
		{"nothing at all", url.Values{}, "Name is too short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, body := post(t, ts, "/printsingle", tc.form)
			if !strings.Contains(body, tc.want) {
				t.Errorf("page does not report %q", tc.want)
			}
			assertStillServing(t, ts)
		})
	}
}

func TestNumberLabelsHappyPath(t *testing.T) {
	ts := newTestServer(t)
	status, body := post(t, ts, "/printnumber", url.Values{"From": {"1"}, "To": {"5"}})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data := fetchGeneratedPDF(t, ts, body)
	if got := countPages(t, data); got != 5 {
		t.Errorf("page count = %d, want 5", got)
	}
}

// A POST with no form fields at all used to panic on r.Form["From"][0].
func TestNumberLabelsEmptyPostDoesNotPanic(t *testing.T) {
	ts := newTestServer(t)
	resp, err := ts.Client().Post(ts.URL+"/printnumber", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), "Enter a whole number") {
		t.Errorf("expected a validation message, got: %s", truncate(string(body), 200))
	}
	assertStillServing(t, ts)
}

func TestNumberLabelsValidation(t *testing.T) {
	ts := newTestServer(t)
	for _, tc := range []struct {
		name string
		form url.Values
		want string
	}{
		{"backwards range", url.Values{"From": {"10"}, "To": {"2"}}, "cannot be less than FROM"},
		{"not a number", url.Values{"From": {"abc"}, "To": {"5"}}, "Enter a whole number in FROM"},
		{"zero", url.Values{"From": {"0"}, "To": {"5"}}, "must be 1 or more"},
		{"absurd range", url.Values{"From": {"1"}, "To": {"999999"}}, "the most in one go is"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, body := post(t, ts, "/printnumber", tc.form)
			if !strings.Contains(body, tc.want) {
				t.Errorf("page does not report %q", tc.want)
			}
			assertStillServing(t, ts)
		})
	}
}

// postCSVs uploads two CSVs the way the browser form does.
func postCSVs(t *testing.T, ts *httptest.Server, moorings, boats [][]string, omit string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	write := func(field, filename string, rows [][]string) {
		if omit == field {
			return
		}
		part, err := mw.CreateFormFile(field, filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if err := csv.NewWriter(part).WriteAll(rows); err != nil {
			t.Fatalf("writing csv: %v", err)
		}
	}
	write("BerthFile", "moorings.csv", moorings)
	write("BoatFile", "boats.csv", boats)
	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart: %v", err)
	}

	resp, err := ts.Client().Post(ts.URL+"/printbulk", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("POST /printbulk: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// currentMoorings builds rows valid at time.Now(), since the handler uses the
// real clock.
func currentMoorings() [][]string {
	return [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "01/Jan/2020", "31/Dec/2099"),
		historicMooringRow("12", "A", "B2", "01/Jan/2020", "31/Dec/2099"),
	}
}

func currentBoats() [][]string {
	return [][]string{
		historicHeader(45),
		historicBoatRow("B1", "Solo:1246", "", "Zoe Adams"),
		historicBoatRow("B2", "Laser:9", "", "Al Baker"),
	}
}

func TestBulkHappyPath(t *testing.T) {
	ts := newTestServer(t)
	status, body := postCSVs(t, ts, currentMoorings(), currentBoats(), "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data := fetchGeneratedPDF(t, ts, body)
	if got := countPages(t, data); got != 4 {
		t.Errorf("page count = %d, want 4 (2 moorings x 2 copies)", got)
	}
	if !strings.Contains(body, "Columns used:") {
		t.Error("page should report which columns it used")
	}
}

// This is the request that killed the server: one row with a blank end date.
func TestBulkBlankEndDateDoesNotKillServer(t *testing.T) {
	ts := newTestServer(t)
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "01/Jan/2020", ""),
	}
	status, body := postCSVs(t, ts, moorings, currentBoats(), "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if data := fetchGeneratedPDF(t, ts, body); countPages(t, data) != 2 {
		t.Error("an open-ended licence should still produce its 2 labels")
	}
	assertStillServing(t, ts)
}

func TestBulkBadDateReportsAndSurvives(t *testing.T) {
	ts := newTestServer(t)
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "the 3rd of never", "31/Dec/2099"),
		historicMooringRow("12", "A", "B2", "01/Jan/2020", "31/Dec/2099"),
	}
	_, body := postCSVs(t, ts, moorings, currentBoats(), "")
	if !strings.Contains(body, "Some rows were skipped") {
		t.Error("page should warn about the skipped row")
	}
	if !strings.Contains(body, "Line 2") {
		t.Error("warning should name the offending line")
	}
	assertStillServing(t, ts)
}

func TestBulkMissingFiles(t *testing.T) {
	ts := newTestServer(t)
	for _, omit := range []string{"BerthFile", "BoatFile"} {
		t.Run("omit "+omit, func(t *testing.T) {
			_, body := postCSVs(t, ts, currentMoorings(), currentBoats(), omit)
			if !strings.Contains(body, "no file was chosen") {
				t.Errorf("expected a missing-file message for %s", omit)
			}
			assertStillServing(t, ts)
		})
	}
}

func TestBulkNoCurrentLicences(t *testing.T) {
	ts := newTestServer(t)
	moorings := [][]string{
		historicHeader(18),
		historicMooringRow("147", "C", "B1", "01/Jan/2001", "31/Dec/2002"),
	}
	_, body := postCSVs(t, ts, moorings, currentBoats(), "")
	if !strings.Contains(body, "No current licences found") {
		t.Error("expected a clear message when nothing is in date")
	}
	assertStillServing(t, ts)
}

// Excel writes a BOM, which would otherwise corrupt the first header name.
func TestBulkToleratesExcelBOM(t *testing.T) {
	ts := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, _ := mw.CreateFormFile("BerthFile", "moorings.csv")
	part.Write([]byte{0xEF, 0xBB, 0xBF})
	csv.NewWriter(part).WriteAll(currentMoorings())

	part2, _ := mw.CreateFormFile("BoatFile", "boats.csv")
	csv.NewWriter(part2).WriteAll(currentBoats())
	mw.Close()

	resp, err := ts.Client().Post(ts.URL+"/printbulk", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !labelURLRe.MatchString(string(body)) {
		t.Error("a BOM-prefixed CSV should still produce labels")
	}
}

func TestExpiredPDFLinkIs404(t *testing.T) {
	ts := newTestServer(t)
	resp, err := ts.Client().Get(ts.URL + "/label/deadbeef.pdf")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDownloadLinkSetsAttachmentHeader(t *testing.T) {
	ts := newTestServer(t)
	_, body := post(t, ts, "/printnumber", url.Values{"From": {"1"}, "To": {"2"}})
	link := labelURLRe.FindString(body)
	if link == "" {
		t.Fatal("no PDF link on page")
	}
	resp, err := ts.Client().Get(ts.URL + link + "?download=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", cd)
	}
}

func TestHealthz(t *testing.T) {
	ts := newTestServer(t)
	resp, err := ts.Client().Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestGetOnPostEndpointRedirects(t *testing.T) {
	ts := newTestServer(t)
	client := *ts.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	resp, err := client.Get(ts.URL + "/printsingle")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", resp.StatusCode)
	}
}

func TestPDFStoreEvictsOldest(t *testing.T) {
	store := newPDFStore(2)
	first := store.put([]byte("one"))
	second := store.put([]byte("two"))
	third := store.put([]byte("three"))

	if _, ok := store.get(first); ok {
		t.Error("the oldest PDF should have been evicted")
	}
	for _, id := range []string{second, third} {
		if _, ok := store.get(id); !ok {
			t.Errorf("recent PDF %s should still be held", id)
		}
	}
}

func TestPDFStoreIDsAreUnguessable(t *testing.T) {
	store := newPDFStore(4)
	a, b := store.put([]byte("a")), store.put([]byte("b"))
	if a == b {
		t.Fatal("ids must be unique")
	}
	if len(a) < 16 {
		t.Errorf("id %q is too short to be unguessable", a)
	}
}
