package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Fetching the mooring export straight from SCM, so a volunteer does not have
// to drive the export screen by hand.
//
// SCM is a Rails application. The flow, captured from a real session on
// 16 Aug 2026, is:
//
//	GET  /login                  -> session cookie and an authenticity_token
//	POST /users/login            -> user[email], user[password]
//	GET  /moorings/export_setup  -> the export form's own authenticity_token
//	POST /moorings/export_actual -> fields[] per column, responds with the CSV
//
// Credentials appear in exactly one place: the body of the login POST. They are
// never put in a URL, never logged, and never reach the rendered page — see
// errSCMLogin and the tests that assert it.

const (
	// defaultSCMBaseURL is Papercourt's own SCM tenant. It is a default rather
	// than a constant so a test (or another club) can point elsewhere.
	defaultSCMBaseURL = "https://papercourtsc.clubmin.net"

	// scmFetchTimeout covers the whole four-request conversation. The export
	// itself took 7.4s server-side on the 2026 file, so this is deliberately
	// generous; it is still bounded so a hung SCM cannot pin a request open.
	scmFetchTimeout = 90 * time.Second

	// maxSCMResponseBytes bounds what we will read back from SCM. The 2026
	// export is ~100KB.
	maxSCMResponseBytes = 32 << 20
)

// scmExportFields are the columns asked of SCM, and deliberately only the
// columns a label needs. The manual upload route hands this program all 27
// columns including invoice numbers, prices and member IDs; asking for seven
// means none of that ever enters the process. Column resolution still works
// because it matches on header text, not position.
var scmExportFields = []string{
	"moor_name",     // -> "Name",              berthnumber
	"moor_group",    // -> "Group",             bertharea
	"alloc_boat",    // -> "Allocation Boat",   allocationboat
	"alloc_contact", // -> "Allocation Contact", allocationcontact
	"alloc_boat_id", // -> "Allocation Boat ID", boatid
	"alloc_from",    // -> "Allocation From",   start
	"alloc_until",   // -> "Allocation Until",  end
}

// scmCredentials is one set of login details for one request. It is never
// stored: either it comes from the environment and stays there, or it comes
// from the form and is discarded when the handler returns.
type scmCredentials struct {
	Email    string
	Password string
}

// scmConfig is resolved once at startup.
type scmConfig struct {
	BaseURL string
	// Env holds credentials supplied by the environment, if any. When they are
	// absent the page asks for them instead.
	Env      scmCredentials
	FromEnv  bool
	Disabled bool // SCM_FETCH=off
}

func loadSCMConfig() scmConfig {
	cfg := scmConfig{
		BaseURL: strings.TrimRight(os.Getenv("SCM_BASE_URL"), "/"),
		Env: scmCredentials{
			Email:    os.Getenv("SCM_EMAIL"),
			Password: os.Getenv("SCM_PASSWORD"),
		},
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultSCMBaseURL
	}
	cfg.FromEnv = cfg.Env.Email != "" && cfg.Env.Password != ""
	cfg.Disabled = strings.EqualFold(os.Getenv("SCM_FETCH"), "off")
	return cfg
}

// errSCMLogin is returned when SCM rejected the credentials. It is a fixed
// string: whatever SCM says about a failed login, nothing derived from the
// submitted email or password is echoed back to the page.
var errSCMLogin = errors.New("SCM would not accept those login details — check them, or use the upload below instead")

// errSCMSession means we logged in but were bounced back to the login page when
// asking for the export, which is what an account without mooring access looks
// like.
var errSCMSession = errors.New("that SCM account signed in but is not allowed to reach the moorings export — use an account with mooring administration rights, or use the upload below")

// scmClient talks to one SCM instance. A cookie jar carries the session across
// the four requests.
type scmClient struct {
	baseURL string
	http    *http.Client
}

func newSCMClient(baseURL string) (*scmClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("preparing the SCM session: %w", err)
	}
	return &scmClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Jar:     jar,
			Timeout: scmFetchTimeout,
		},
	}, nil
}

// fetchMoorings logs in and returns the mooring export as parsed CSV rows.
func (c *scmClient) fetchMoorings(ctx context.Context, creds scmCredentials) ([][]string, error) {
	loginToken, _, err := c.getPage(ctx, "/login")
	if err != nil {
		return nil, fmt.Errorf("could not reach SCM at %s: %w", c.baseURL, err)
	}

	form := url.Values{
		"utf8":               {"✓"},
		"authenticity_token": {loginToken},
		"user[email]":        {creds.Email},
		"user[password]":     {creds.Password},
		// Explicitly decline "remember me": this session should end with the
		// request, not leave a long-lived cookie anywhere.
		"user[remember]": {"0"},
	}
	resp, err := c.postForm(ctx, "/users/login", form)
	if err != nil {
		// Deliberately not wrapped with the form: it holds the password.
		return nil, fmt.Errorf("could not sign in to SCM at %s: %w", c.baseURL, redactURLError(err))
	}
	_, err = drain(resp)
	if err != nil {
		return nil, fmt.Errorf("could not sign in to SCM at %s: %w", c.baseURL, err)
	}
	if isLoginPage(resp) {
		return nil, errSCMLogin
	}

	exportToken, exportResp, err := c.getPage(ctx, "/moorings/export_setup")
	if err != nil {
		return nil, fmt.Errorf("could not open the SCM export page: %w", err)
	}
	if isLoginPage(exportResp) {
		return nil, errSCMSession
	}
	if exportToken == "" {
		return nil, errors.New("the SCM export page did not look as expected (no form token found) — SCM may have changed; use the upload below instead")
	}

	fields := url.Values{
		"utf8":               {"✓"},
		"authenticity_token": {exportToken},
	}
	for _, f := range scmExportFields {
		fields.Add("fields[]", f)
	}
	csvResp, err := c.postForm(ctx, "/moorings/export_actual", fields)
	if err != nil {
		return nil, fmt.Errorf("could not download the mooring export: %w", err)
	}
	body, err := drain(csvResp)
	if err != nil {
		return nil, fmt.Errorf("could not download the mooring export: %w", err)
	}
	if isLoginPage(csvResp) {
		return nil, errSCMSession
	}
	if !looksLikeCSVResponse(csvResp) {
		return nil, errors.New("SCM returned a web page instead of a CSV file — it may have changed its export screen; use the upload below instead")
	}

	rows, err := parseCSV(body, "the SCM mooring export")
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("SCM returned an export with no rows in it")
	}
	return rows, nil
}

// getPage fetches an HTML page and pulls the Rails authenticity_token out of it.
func (c *scmClient) getPage(ctx context.Context, path string) (string, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", nil, redactURLError(err)
	}
	body, err := drain(resp)
	if err != nil {
		return "", resp, err
	}
	return findAuthenticityToken(body), resp, nil
}

func (c *scmClient) postForm(ctx context.Context, path string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, redactURLError(err)
	}
	return resp, nil
}

// drain reads and closes a response body, bounded.
func drain(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSCMResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading the reply from SCM: %w", err)
	}
	resp.Body = io.NopCloser(strings.NewReader(string(body)))
	return body, nil
}

var authenticityTokenRe = regexp.MustCompile(`name="authenticity_token"[^>]*value="([^"]*)"`)

// findAuthenticityToken pulls Rails' CSRF token out of a form. The attribute
// order varies between SCM's pages, so both orderings are tried.
func findAuthenticityToken(body []byte) string {
	if m := authenticityTokenRe.FindSubmatch(body); m != nil {
		return html.UnescapeString(string(m[1]))
	}
	alt := regexp.MustCompile(`value="([^"]*)"[^>]*name="authenticity_token"`)
	if m := alt.FindSubmatch(body); m != nil {
		return html.UnescapeString(string(m[1]))
	}
	return ""
}

// isLoginPage reports whether a response ended up back at the login screen,
// which is how SCM says both "wrong password" and "not allowed in here".
func isLoginPage(resp *http.Response) bool {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	p := resp.Request.URL.Path
	return p == "/login" || strings.HasPrefix(p, "/login/") || p == "/users/login"
}

// looksLikeCSVResponse distinguishes the export download from an HTML page.
func looksLikeCSVResponse(resp *http.Response) bool {
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Disposition")), "attachment") {
		return true
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	return strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/csv") ||
		strings.HasPrefix(ct, "application/csv")
}

// redactURLError strips the request URL out of net/http errors. SCM's URLs
// never carry credentials, but this program should not be the reason a URL
// ends up in a club volunteer's screenshot either.
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s request failed: %w", ue.Op, ue.Err)
	}
	return err
}
