package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// These tests are mostly about what must be REFUSED. A verifier that accepts
// good tokens but also accepts forged ones is worse than none at all, because
// it invites turning the network protections off.

const (
	testIssuer = "https://testteam.cloudflareaccess.com"
	testAUD    = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testKid    = "test-key-1"
)

type testSigner struct {
	key   *rsa.PrivateKey
	certs *httptest.Server
	// hits counts key-set fetches, to check caching and rate limiting.
	hits int
}

func newTestSigner(t *testing.T) *testSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	s := &testSigner{key: key}

	mux := http.NewServeMux()
	mux.HandleFunc("/cdn-cgi/access/certs", func(w http.ResponseWriter, r *http.Request) {
		s.hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": testKid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   b64(key.N.Bytes()),
				"e":   b64([]byte{0x01, 0x00, 0x01}),
			}},
		})
	})
	s.certs = httptest.NewServer(mux)
	t.Cleanup(s.certs.Close)
	return s
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// sign builds a JWT. Every part is a parameter so a test can corrupt exactly
// one thing and leave the rest valid.
func (s *testSigner) sign(t *testing.T, header, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signing := b64(h) + "." + b64(c)

	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	return signing + "." + b64(sig)
}

func goodHeader() map[string]any {
	return map[string]any{"alg": "RS256", "kid": testKid, "typ": "JWT"}
}

func goodClaims() map[string]any {
	return map[string]any{
		"iss":   testIssuer,
		"aud":   []string{testAUD},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
		"nbf":   time.Now().Add(-time.Minute).Unix(),
		"email": "member@example.invalid",
	}
}

func newTestVerifier(s *testSigner) *accessVerifier {
	v := newAccessVerifier(accessConfig{Issuer: testIssuer, AUD: testAUD, Enforced: true})
	// Point key fetching at the test server while keeping the real issuer, so
	// the issuer claim check is still exercised.
	v.cfg.Issuer = testIssuer
	v.client = s.certs.Client()
	v.certsOverride = s.certs.URL + "/cdn-cgi/access/certs"
	return v
}

func TestAccessAcceptsAValidToken(t *testing.T) {
	s := newTestSigner(t)
	v := newTestVerifier(s)

	if err := v.verify(context.Background(), s.sign(t, goodHeader(), goodClaims())); err != nil {
		t.Fatalf("a valid token was rejected: %v", err)
	}
}

// The table of things that must never be accepted.
func TestAccessRefusesBadTokens(t *testing.T) {
	s := newTestSigner(t)

	// An unrelated key, standing in for an attacker signing their own token.
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	otherSigner := &testSigner{key: other, certs: s.certs}

	cases := []struct {
		name  string
		token func() string
		want  string
	}{
		{
			name:  "not a JWT at all",
			token: func() string { return "obviously-not-a-token" },
			want:  "not a JWT",
		},
		{
			name: "alg none, unsigned",
			token: func() string {
				h, _ := json.Marshal(map[string]any{"alg": "none", "kid": testKid})
				c, _ := json.Marshal(goodClaims())
				return b64(h) + "." + b64(c) + "."
			},
			want: "unexpected token algorithm",
		},
		{
			name: "alg swapped to HMAC",
			token: func() string {
				h, _ := json.Marshal(map[string]any{"alg": "HS256", "kid": testKid})
				c, _ := json.Marshal(goodClaims())
				return b64(h) + "." + b64(c) + ".c2ln"
			},
			want: "unexpected token algorithm",
		},
		{
			name:  "signed by someone else's key",
			token: func() string { return otherSigner.sign(t, goodHeader(), goodClaims()) },
			want:  "signature does not verify",
		},
		{
			name: "claims tampered with after signing",
			token: func() string {
				tok := s.sign(t, goodHeader(), goodClaims())
				parts := strings.Split(tok, ".")
				swapped, _ := json.Marshal(map[string]any{
					"iss": testIssuer, "aud": []string{testAUD},
					"exp":   time.Now().Add(100 * time.Hour).Unix(),
					"email": "attacker@example.invalid",
				})
				return parts[0] + "." + b64(swapped) + "." + parts[2]
			},
			want: "signature does not verify",
		},
		{
			name: "expired",
			token: func() string {
				c := goodClaims()
				c["exp"] = time.Now().Add(-time.Minute).Unix()
				return s.sign(t, goodHeader(), c)
			},
			want: "expired",
		},
		{
			name: "no expiry at all",
			token: func() string {
				c := goodClaims()
				delete(c, "exp")
				return s.sign(t, goodHeader(), c)
			},
			want: "no expiry",
		},
		{
			name: "not valid yet",
			token: func() string {
				c := goodClaims()
				c["nbf"] = time.Now().Add(time.Hour).Unix()
				return s.sign(t, goodHeader(), c)
			},
			want: "not valid yet",
		},
		{
			name: "issued by a different Cloudflare team",
			token: func() string {
				c := goodClaims()
				c["iss"] = "https://someoneelse.cloudflareaccess.com"
				return s.sign(t, goodHeader(), c)
			},
			want: "not this team",
		},
		{
			name: "for a different Access application",
			token: func() string {
				c := goodClaims()
				c["aud"] = []string{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}
				return s.sign(t, goodHeader(), c)
			},
			want: "different Access application",
		},
		{
			name: "names no signing key",
			token: func() string {
				h := goodHeader()
				delete(h, "kid")
				return s.sign(t, h, goodClaims())
			},
			want: "names no signing key",
		},
		{
			name: "signed by an unknown key id",
			token: func() string {
				h := goodHeader()
				h["kid"] = "some-other-key"
				return s.sign(t, h, goodClaims())
			},
			want: "unknown key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVerifier(s)
			err := v.verify(context.Background(), tc.token())
			if err == nil {
				t.Fatalf("ACCEPTED a token that must be refused (%s)", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// The middleware is what actually stands between the internet and the data.
func TestAccessMiddlewareBlocksAndAllows(t *testing.T) {
	s := newTestSigner(t)
	v := newTestVerifier(s)

	reached := false
	guarded := v.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		fmt.Fprintln(w, "the labels page")
	}))

	// No token: this is the direct-to-origin request that bypasses Cloudflare.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a request that skipped Cloudflare", rec.Code)
	}
	if reached {
		t.Fatal("an unauthenticated request reached the application")
	}

	// A valid token in the header, as Cloudflare sends it.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(accessHeader, s.sign(t, goodHeader(), goodClaims()))
	rec = httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a properly signed request", rec.Code)
	}
	if !reached {
		t.Fatal("a valid request did not reach the application")
	}

	// The same token in the cookie, as a browser carries it.
	reached = false
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: accessCookie, Value: s.sign(t, goodHeader(), goodClaims())})
	rec = httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !reached {
		t.Fatalf("cookie-carried token rejected: status %d", rec.Code)
	}
}

// Every route must be guarded — it would be easy to protect the form and forget
// the URL the generated PDF is served from.
func TestAccessGuardsEveryRouteButHealthz(t *testing.T) {
	s := newTestSigner(t)
	v := newTestVerifier(s)

	srv, err := newServer()
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	guarded := v.middleware(srv.routes())

	for _, path := range []string{"/", "/printsingle", "/printbulk", "/printnumber", "/printfetch", "/label/abc.pdf"} {
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s returned %d without a token, want 403", path, rec.Code)
		}
	}

	// The container health check runs inside the container and never passes
	// through Cloudflare, so it must stay reachable or the deploy never goes
	// healthy.
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz returned %d, want 200 — the container would never become healthy", rec.Code)
	}
}

// A denial must not echo the token: it is a bearer credential and names a real
// person.
func TestAccessDenialDoesNotEchoTheToken(t *testing.T) {
	s := newTestSigner(t)
	v := newTestVerifier(s)
	token := s.sign(t, goodHeader(), goodClaims())

	guarded := v.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(accessHeader, token+"tampered")
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if strings.Contains(rec.Body.String(), token[:40]) {
		t.Fatal("the response echoed the token back")
	}
}

func TestAccessCachesTheKeySet(t *testing.T) {
	s := newTestSigner(t)
	v := newTestVerifier(s)

	for i := 0; i < 5; i++ {
		if err := v.verify(context.Background(), s.sign(t, goodHeader(), goodClaims())); err != nil {
			t.Fatalf("verify %d: %v", i, err)
		}
	}
	if s.hits != 1 {
		t.Errorf("fetched the key set %d times for 5 requests, want 1", s.hits)
	}
}

// A stream of junk tokens naming unknown keys must not turn into a stream of
// outbound requests to Cloudflare.
func TestAccessRateLimitsKeyRefetch(t *testing.T) {
	s := newTestSigner(t)
	v := newTestVerifier(s)

	h := goodHeader()
	h["kid"] = "nonexistent"
	for i := 0; i < 10; i++ {
		if err := v.verify(context.Background(), s.sign(t, h, goodClaims())); err == nil {
			t.Fatal("accepted a token signed by an unknown key")
		}
	}
	if s.hits > 1 {
		t.Errorf("fetched the key set %d times for 10 junk tokens, want at most 1", s.hits)
	}
}

func TestLoadAccessConfig(t *testing.T) {
	t.Run("off when neither is set", func(t *testing.T) {
		t.Setenv("CF_ACCESS_TEAM_DOMAIN", "")
		t.Setenv("CF_ACCESS_AUD", "")
		cfg, err := loadAccessConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Enforced {
			t.Error("should not be enforced with nothing configured")
		}
	})

	// Half a configuration must stop the program, not silently serve the app to
	// the internet.
	for _, tc := range []struct{ team, aud string }{
		{"testteam", ""},
		{"", testAUD},
	} {
		t.Run(fmt.Sprintf("refuses team=%q aud=%q", tc.team, tc.aud), func(t *testing.T) {
			t.Setenv("CF_ACCESS_TEAM_DOMAIN", tc.team)
			t.Setenv("CF_ACCESS_AUD", tc.aud)
			if _, err := loadAccessConfig(); err == nil {
				t.Fatal("a half-configuration was accepted")
			}
		})
	}

	for _, tc := range []struct{ in, want string }{
		{"testteam", "https://testteam.cloudflareaccess.com"},
		{"testteam.cloudflareaccess.com", "https://testteam.cloudflareaccess.com"},
		{"https://testteam.cloudflareaccess.com", "https://testteam.cloudflareaccess.com"},
		{"https://testteam.cloudflareaccess.com/", "https://testteam.cloudflareaccess.com"},
	} {
		t.Run("issuer from "+tc.in, func(t *testing.T) {
			t.Setenv("CF_ACCESS_TEAM_DOMAIN", tc.in)
			t.Setenv("CF_ACCESS_AUD", testAUD)
			cfg, err := loadAccessConfig()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Issuer != tc.want {
				t.Errorf("issuer = %q, want %q", cfg.Issuer, tc.want)
			}
			if !cfg.Enforced {
				t.Error("should be enforced")
			}
		})
	}
}

func TestAudienceSetAcceptsStringOrList(t *testing.T) {
	var a audienceSet
	if err := json.Unmarshal([]byte(`"one"`), &a); err != nil || !a.contains("one") {
		t.Errorf("single string audience not handled: %v %v", a, err)
	}
	if err := json.Unmarshal([]byte(`["one","two"]`), &a); err != nil || !a.contains("two") {
		t.Errorf("list audience not handled: %v %v", a, err)
	}
	if a.contains("three") {
		t.Error("contains matched something absent")
	}
}
