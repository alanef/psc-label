package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// A stand-in for SCM. It mimics the real conversation captured on 16 Aug 2026 —
// Rails authenticity tokens, a session cookie, and a CSV served as an
// attachment — without any of the real data. There is deliberately no fixture
// of a real export anywhere in this repository.

const (
	fakeEmail    = "moorings@example.invalid"
	fakePassword = "correct horse battery staple"
	fakeToken    = "tOkEn+/=" // exercises the base64 characters Rails emits
)

// fakeSCMHeader is the header SCM returns for the seven fields this program
// asks for: the real header text, in the order the reduced field set produces.
var fakeSCMHeader = "Name,Group,Allocation Boat,Allocation Contact,Allocation Boat ID,Allocation From,Allocation Until"

type fakeSCM struct {
	t *testing.T

	// knobs
	rejectLogin  bool
	denyMoorings bool // signs in, but the export page bounces to /login
	htmlInstead  bool // returns a web page where the CSV should be
	body         string

	// what the fake saw
	gotFields   []string
	gotEmail    string
	loginTokens []string
	requests    []string
}

func (f *fakeSCM) start() *httptest.Server {
	f.t.Helper()
	mux := http.NewServeMux()

	loginPage := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<form id="login-form" action="/users/login" method="post">
<input name="utf8" type="hidden" value="&#x2713;" />
<input type="hidden" name="authenticity_token" value="%s" />
<input type="text" name="user[email]" />
<input type="password" name="user[password]" />
</form>`, "tOkEn+/&#61;")
	}

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, "GET /login")
		http.SetCookie(w, &http.Cookie{Name: "bs_session_id", Value: "anonymous", Path: "/"})
		loginPage(w)
	})

	mux.HandleFunc("/users/login", func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, "POST /users/login")
		if err := r.ParseForm(); err != nil {
			f.t.Errorf("login form did not parse: %v", err)
		}
		f.gotEmail = r.PostFormValue("user[email]")
		f.loginTokens = append(f.loginTokens, r.PostFormValue("authenticity_token"))

		ok := r.PostFormValue("user[email]") == fakeEmail &&
			r.PostFormValue("user[password]") == fakePassword &&
			r.PostFormValue("authenticity_token") == fakeToken
		if f.rejectLogin || !ok {
			// Rails re-renders the login page on a bad password.
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "bs_session_id", Value: "signed-in", Path: "/"})
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})

	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "<html><body>signed in</body></html>")
	})

	mux.HandleFunc("/moorings/export_setup", func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, "GET /moorings/export_setup")
		if f.denyMoorings || !f.signedIn(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<form action="/moorings/export_actual" method="post">
<input type="hidden" name="authenticity_token" value="%s" />
</form>`, "tOkEn+/&#61;")
	})

	mux.HandleFunc("/moorings/export_actual", func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, "POST /moorings/export_actual")
		if !f.signedIn(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if err := r.ParseForm(); err != nil {
			f.t.Errorf("export form did not parse: %v", err)
		}
		f.gotFields = r.PostForm["fields[]"]
		if got := r.PostFormValue("authenticity_token"); got != fakeToken {
			f.t.Errorf("export sent authenticity_token %q, want %q", got, fakeToken)
		}
		if f.htmlInstead {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintln(w, "<html><body>Something went wrong</body></html>")
			return
		}
		w.Header().Set("Content-Type", "text/plain; header=present")
		w.Header().Set("Content-Disposition", `attachment; filename=moorings20260816.csv`)
		fmt.Fprint(w, f.body)
	})

	srv := httptest.NewServer(mux)
	f.t.Cleanup(srv.Close)
	return srv
}

func (f *fakeSCM) signedIn(r *http.Request) bool {
	c, err := r.Cookie("bs_session_id")
	return err == nil && c.Value == "signed-in"
}

// fakeExport builds a small synthetic export in the shape SCM returns for the
// seven requested fields.
func fakeExport(rows ...string) string {
	return fakeSCMHeader + "\n" + strings.Join(rows, "\n") + "\n"
}

func newFake(t *testing.T) (*fakeSCM, *httptest.Server) {
	t.Helper()
	f := &fakeSCM{
		t: t,
		body: fakeExport(
			`145,C,Solo:4669,A Member,38321,01/Jan/2026,31/Dec/2026`,
			`127,C,Scorpion: 666,Another Member,305185,01/Feb/2026,31/Dec/2026`,
		),
	}
	return f, f.start()
}

func TestFetchMooringsHappyPath(t *testing.T) {
	f, srv := newFake(t)

	client, err := newSCMClient(srv.URL)
	if err != nil {
		t.Fatalf("newSCMClient: %v", err)
	}
	rows, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, fakePassword})
	if err != nil {
		t.Fatalf("fetchMoorings: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (header plus two)", len(rows))
	}
	if got, want := strings.Join(rows[0], ","), fakeSCMHeader; got != want {
		t.Errorf("header = %q, want %q", got, want)
	}

	// The four-request conversation, in order.
	want := []string{"GET /login", "POST /users/login", "GET /moorings/export_setup", "POST /moorings/export_actual"}
	if strings.Join(f.requests, "|") != strings.Join(want, "|") {
		t.Errorf("requests = %v, want %v", f.requests, want)
	}
}

// The reduced field set is the privacy property: this program must never ask
// SCM for invoices, prices, member IDs or coordinates.
func TestFetchAsksOnlyForLabelFields(t *testing.T) {
	f, srv := newFake(t)
	client, _ := newSCMClient(srv.URL)
	if _, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, fakePassword}); err != nil {
		t.Fatalf("fetchMoorings: %v", err)
	}

	want := []string{"moor_name", "moor_group", "alloc_boat", "alloc_contact", "alloc_boat_id", "alloc_from", "alloc_until"}
	if strings.Join(f.gotFields, ",") != strings.Join(want, ",") {
		t.Fatalf("requested fields = %v, want %v", f.gotFields, want)
	}
	for _, unwanted := range []string{"alloc_invoice", "alloc_price", "alloc_contact_id", "latitude", "longitude", "alloc_outstanding"} {
		for _, got := range f.gotFields {
			if got == unwanted {
				t.Errorf("asked SCM for %q, which no label needs", unwanted)
			}
		}
	}
}

// The whole point of the flow: the CSV that comes back must drive the same
// label building as an uploaded file, with no boats file at all.
func TestFetchedExportBuildsLabels(t *testing.T) {
	_, srv := newFake(t)
	client, _ := newSCMClient(srv.URL)
	rows, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, fakePassword})
	if err != nil {
		t.Fatalf("fetchMoorings: %v", err)
	}

	result, err := buildLabels(rows, nil, testNow)
	if err != nil {
		t.Fatalf("buildLabels on the fetched export: %v", err)
	}
	if len(result.Labels) != 2 {
		t.Fatalf("got %d labels from the fetched export, want 2 (warnings: %v)", len(result.Labels), result.Warnings)
	}
	if result.Labels[0].Berth == "" || result.Labels[0].Name == "" {
		t.Errorf("label came out blank: %+v", result.Labels[0])
	}
}

func TestFetchRejectedLogin(t *testing.T) {
	f, srv := newFake(t)
	f.rejectLogin = true

	client, _ := newSCMClient(srv.URL)
	_, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, "wrong"})
	if err == nil {
		t.Fatal("expected an error for a rejected login")
	}
	if !strings.Contains(err.Error(), "would not accept those login details") {
		t.Errorf("error = %q, want the friendly login message", err)
	}
	if !strings.Contains(err.Error(), "upload below") {
		t.Errorf("error %q should point at the upload fallback", err)
	}
}

func TestFetchAccountWithoutMooringAccess(t *testing.T) {
	f, srv := newFake(t)
	f.denyMoorings = true

	client, _ := newSCMClient(srv.URL)
	_, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, fakePassword})
	if err == nil {
		t.Fatal("expected an error when the export page bounces to login")
	}
	if !strings.Contains(err.Error(), "not allowed to reach the moorings export") {
		t.Errorf("error = %q, want the permissions message", err)
	}
}

func TestFetchHTMLInsteadOfCSV(t *testing.T) {
	f, srv := newFake(t)
	f.htmlInstead = true

	client, _ := newSCMClient(srv.URL)
	_, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, fakePassword})
	if err == nil {
		t.Fatal("expected an error when SCM returns a page instead of a CSV")
	}
	if !strings.Contains(err.Error(), "web page instead of a CSV") {
		t.Errorf("error = %q, want the wrong-content-type message", err)
	}
}

// No error this program produces may contain the password, whatever went wrong.
func TestFetchErrorsNeverContainCredentials(t *testing.T) {
	cases := map[string]func(*fakeSCM){
		"rejected login":    func(f *fakeSCM) { f.rejectLogin = true },
		"no mooring access": func(f *fakeSCM) { f.denyMoorings = true },
		"html instead":      func(f *fakeSCM) { f.htmlInstead = true },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f, srv := newFake(t)
			setup(f)
			client, _ := newSCMClient(srv.URL)
			_, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, fakePassword})
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), fakePassword) {
				t.Fatalf("error leaked the password: %q", err)
			}
			if strings.Contains(logSafeFetchError(err), fakePassword) {
				t.Fatalf("log line leaked the password: %q", logSafeFetchError(err))
			}
		})
	}
}

// A rejected login must not put the submitted email in the log either: SCM's
// own error page is free to quote it back, and we do not pass that through.
func TestRejectedLoginIsNotLoggedInDetail(t *testing.T) {
	f, srv := newFake(t)
	f.rejectLogin = true
	client, _ := newSCMClient(srv.URL)
	_, err := client.fetchMoorings(context.Background(), scmCredentials{fakeEmail, fakePassword})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := logSafeFetchError(err); got != "SCM rejected the credentials" {
		t.Errorf("log line = %q, want the fixed rejection string", got)
	}
}

// Rails tokens contain +, / and =, and are HTML-escaped in the page. Getting
// this wrong fails CSRF on the export and is awkward to diagnose in the wild.
func TestFindAuthenticityTokenUnescapes(t *testing.T) {
	for _, tc := range []struct {
		name, html, want string
	}{
		{"name then value", `<input type="hidden" name="authenticity_token" value="ab+/&#61;" />`, "ab+/="},
		{"value then name", `<input value="ab+/&#61;" name="authenticity_token" />`, "ab+/="},
		{"absent", `<input name="something_else" value="x" />`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := findAuthenticityToken([]byte(tc.html)); got != tc.want {
				t.Errorf("findAuthenticityToken = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRedactURLError(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "https://scm.example/users/login?secret=x", Err: fmt.Errorf("connection refused")}
	got := redactURLError(err).Error()
	if strings.Contains(got, "scm.example") || strings.Contains(got, "secret") {
		t.Errorf("redactURLError kept the URL: %q", got)
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("redactURLError lost the cause: %q", got)
	}
}

func TestLoadSCMConfigDefaultsAndEnv(t *testing.T) {
	t.Setenv("SCM_BASE_URL", "")
	t.Setenv("SCM_EMAIL", "")
	t.Setenv("SCM_PASSWORD", "")
	t.Setenv("SCM_FETCH", "")

	cfg := loadSCMConfig()
	if cfg.BaseURL != defaultSCMBaseURL {
		t.Errorf("BaseURL = %q, want the Papercourt default", cfg.BaseURL)
	}
	if cfg.FromEnv {
		t.Error("FromEnv should be false with no credentials in the environment")
	}
	if cfg.Disabled {
		t.Error("fetching should be on by default")
	}

	t.Setenv("SCM_BASE_URL", "https://elsewhere.example/")
	t.Setenv("SCM_EMAIL", fakeEmail)
	t.Setenv("SCM_PASSWORD", fakePassword)
	t.Setenv("SCM_FETCH", "OFF")
	cfg = loadSCMConfig()
	if cfg.BaseURL != "https://elsewhere.example" {
		t.Errorf("BaseURL = %q, want the trailing slash trimmed", cfg.BaseURL)
	}
	if !cfg.FromEnv {
		t.Error("FromEnv should be true when both credentials are set")
	}
	if !cfg.Disabled {
		t.Error("SCM_FETCH=OFF should disable fetching, case-insensitively")
	}

	// One credential alone is not enough — half-configured must fall back to
	// asking on the page rather than sending an empty password to SCM.
	t.Setenv("SCM_PASSWORD", "")
	if loadSCMConfig().FromEnv {
		t.Error("FromEnv should be false when only the email is set")
	}
}
