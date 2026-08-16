package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Enforcing Cloudflare Access at the application, not just at the edge.
//
// Zero Trust in front of this app only helps if every request actually goes
// through Cloudflare. A Coolify host publishes its proxy on 80/443 to the whole
// internet, so anyone who knows the hostname can point it at the origin IP and
// reach the app directly, with Access never consulted:
//
//	curl --resolve host:443:<origin-ip> https://host/
//
// Cloudflare signs a JWT into every request it does let through. Checking that
// signature here means a request that skipped the edge has no valid token and
// gets a 403 — the port being open stops mattering.
//
// This is deliberately fail-closed: if it is switched on and anything is wrong,
// requests are refused rather than allowed through.

const (
	// accessHeader is where Cloudflare puts the signed token. The cookie is the
	// same token, and is what a browser navigating directly will carry.
	accessHeader = "Cf-Access-Jwt-Assertion"
	accessCookie = "CF_Authorization"

	// jwksTTL is how long a fetched key set is trusted before refetching.
	// Cloudflare rotates keys roughly every six weeks.
	jwksTTL = 1 * time.Hour

	// jwksMinRefetch rate-limits refetching on an unknown key id, so a stream
	// of junk tokens cannot turn into a stream of outbound requests.
	jwksMinRefetch = 1 * time.Minute
)

// accessConfig describes whether and how to enforce Cloudflare Access.
type accessConfig struct {
	Issuer   string // https://<team>.cloudflareaccess.com
	AUD      string // the Access application's Application Audience tag
	Enforced bool
}

// loadAccessConfig reads the configuration. A half-configuration is a startup
// error rather than a silent fallback to "off": someone who sets one of the two
// variables is trying to lock the app down, and quietly serving it to the
// internet instead is the worst possible reading of that intent.
func loadAccessConfig() (accessConfig, error) {
	team := strings.TrimSpace(os.Getenv("CF_ACCESS_TEAM_DOMAIN"))
	aud := strings.TrimSpace(os.Getenv("CF_ACCESS_AUD"))

	if team == "" && aud == "" {
		return accessConfig{}, nil
	}
	if team == "" || aud == "" {
		return accessConfig{}, errors.New("CF_ACCESS_TEAM_DOMAIN and CF_ACCESS_AUD must be set together — setting only one would leave the app open to anyone who reaches the origin directly")
	}

	issuer := team
	if !strings.HasPrefix(issuer, "https://") && !strings.HasPrefix(issuer, "http://") {
		// Accept a bare team name as well as a full URL.
		if !strings.Contains(issuer, ".") {
			issuer += ".cloudflareaccess.com"
		}
		issuer = "https://" + issuer
	}
	issuer = strings.TrimRight(issuer, "/")

	return accessConfig{Issuer: issuer, AUD: aud, Enforced: true}, nil
}

// accessVerifier checks Cloudflare Access tokens against the team's public keys.
type accessVerifier struct {
	cfg    accessConfig
	client *http.Client

	// certsOverride points key fetching somewhere other than the real issuer.
	// Only the tests set it, so that claim checking still runs against the real
	// issuer string while the keys come from a local server.
	certsOverride string

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func newAccessVerifier(cfg accessConfig) *accessVerifier {
	return &accessVerifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
		keys:   map[string]*rsa.PublicKey{},
	}
}

func (v *accessVerifier) certsURL() string {
	if v.certsOverride != "" {
		return v.certsOverride
	}
	return v.cfg.Issuer + "/cdn-cgi/access/certs"
}

// middleware refuses any request without a valid Access token. /healthz is
// exempt: Docker's HEALTHCHECK runs inside the container and never passes
// through Cloudflare, and it reveals nothing.
func (v *accessVerifier) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get(accessHeader)
		if token == "" {
			if c, err := r.Cookie(accessCookie); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			v.deny(w, r, "no Cloudflare Access token on the request")
			return
		}
		if err := v.verify(r.Context(), token); err != nil {
			v.deny(w, r, err.Error())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// deny logs why and says as little as possible to the caller. The token itself
// is never logged: it identifies a real person and is a bearer credential.
func (v *accessVerifier) deny(w http.ResponseWriter, r *http.Request, reason string) {
	log.Printf("access denied for %s %s: %s", r.Method, r.URL.Path, reason)
	http.Error(w, "This page is behind Cloudflare Access. Open it through the proper address and sign in.", http.StatusForbidden)
}

// verify checks the signature and the claims. It is written to fail closed:
// every path that is not an explicit success returns an error.
func (v *accessVerifier) verify(ctx context.Context, token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("token is not a JWT")
	}

	headerJSON, err := base64URL(parts[0])
	if err != nil {
		return fmt.Errorf("unreadable token header: %w", err)
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return fmt.Errorf("unreadable token header: %w", err)
	}
	// Pinned, not taken from the token. Trusting the token's own "alg" is the
	// classic JWT break: "none" would skip verification entirely, and an HMAC
	// algorithm would let the public key be used as the signing secret.
	if hdr.Alg != "RS256" {
		return fmt.Errorf("unexpected token algorithm %q, want RS256", hdr.Alg)
	}
	if hdr.Kid == "" {
		return errors.New("token names no signing key")
	}

	key, err := v.keyFor(ctx, hdr.Kid)
	if err != nil {
		return err
	}

	sig, err := base64URL(parts[2])
	if err != nil {
		return fmt.Errorf("unreadable token signature: %w", err)
	}
	signed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, signed[:], sig); err != nil {
		return errors.New("token signature does not verify")
	}

	payload, err := base64URL(parts[1])
	if err != nil {
		return fmt.Errorf("unreadable token body: %w", err)
	}
	var claims struct {
		Issuer    string      `json:"iss"`
		Audience  audienceSet `json:"aud"`
		Expiry    int64       `json:"exp"`
		NotBefore int64       `json:"nbf"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("unreadable token body: %w", err)
	}

	if claims.Issuer != v.cfg.Issuer {
		return fmt.Errorf("token was issued by %q, not this team", claims.Issuer)
	}
	if !claims.Audience.contains(v.cfg.AUD) {
		return errors.New("token is for a different Access application")
	}
	now := time.Now()
	if claims.Expiry == 0 {
		return errors.New("token has no expiry")
	}
	if now.After(time.Unix(claims.Expiry, 0)) {
		return errors.New("token has expired")
	}
	if claims.NotBefore != 0 && now.Before(time.Unix(claims.NotBefore, 0)) {
		return errors.New("token is not valid yet")
	}
	return nil
}

// keyFor returns the public key with this id, fetching the key set if it is
// unknown or stale.
func (v *accessVerifier) keyFor(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Since(v.fetchedAt) < jwksTTL
	lastFetch := v.fetchedAt
	v.mu.RUnlock()

	if ok && fresh {
		return key, nil
	}
	// An unknown key id usually means Cloudflare rotated. Refetch, but not more
	// often than jwksMinRefetch, so junk tokens cannot drive outbound traffic.
	if !ok && time.Since(lastFetch) < jwksMinRefetch {
		return nil, errors.New("token signed by an unknown key")
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, errors.New("token signed by an unknown key")
	}
	return key, nil
}

func (v *accessVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL(), nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not fetch the Access signing keys: %w", redactURLError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching the Access signing keys returned status %d", resp.StatusCode)
	}

	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("could not read the Access signing keys: %w", err)
	}

	keys := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		nBytes, err := base64URL(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64URL(k.E)
		if err != nil {
			continue
		}
		e := new(big.Int).SetBytes(eBytes)
		if !e.IsInt64() || e.Int64() < 3 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(e.Int64())}
	}
	if len(keys) == 0 {
		return errors.New("the Access key set contained no usable keys")
	}

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

// audienceSet handles "aud" being either a string or a list of strings, which
// the JWT spec allows and Cloudflare uses the list form of.
type audienceSet []string

func (a *audienceSet) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*a = audienceSet{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a audienceSet) contains(want string) bool {
	for _, got := range a {
		// Constant time is not needed: the audience tag is not a secret, it is
		// published in the Access dashboard and sent in every token.
		if got == want {
			return true
		}
	}
	return false
}

func base64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}
