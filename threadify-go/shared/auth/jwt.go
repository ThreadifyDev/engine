package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenClaims struct {
	Sub           string
	AuthUserID    string // Supabase auth_user_id (same as Sub)
	UserID        string // Internal Threadify user ID from user_metadata.threadify_user_id
	CompanyID     string
	Email         string
	EmailVerified bool
	Roles         []string
	ExpiresAt     time.Time
}

type JWKSVerifier struct {
	jwksURL  string
	audience string
	issuer   string
	timeout  time.Duration
	cacheTTL time.Duration

	mu       sync.RWMutex
	keys     map[string]interface{}
	cachedAt time.Time

	httpClient *http.Client
}

func NewJWKSVerifier(jwksURL, audience, issuer string) *JWKSVerifier {
	return &JWKSVerifier{
		jwksURL:    strings.TrimRight(strings.TrimSpace(jwksURL), "/"),
		audience:   strings.TrimSpace(audience),
		issuer:     strings.TrimSpace(issuer),
		timeout:    10 * time.Second,
		cacheTTL:   10 * time.Minute,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (v *JWKSVerifier) Verify(ctx context.Context, tokenString string) (*TokenClaims, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("empty token")
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{
		jwt.SigningMethodRS256.Alg(),
		jwt.SigningMethodES256.Alg(),
	}))

	var mapClaims jwt.MapClaims
	token, err := parser.ParseWithClaims(tokenString, &mapClaims, func(token *jwt.Token) (interface{}, error) {
		kid, _ := token.Header["kid"].(string)
		return v.getKey(ctx, strings.TrimSpace(kid))
	})

	if err != nil || token == nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if v.issuer != "" && claimStr(mapClaims, "iss") != v.issuer {
		return nil, errors.New("invalid token issuer")
	}

	if v.audience != "" && !audienceContains(mapClaims["aud"], v.audience) {
		return nil, errors.New("invalid token audience")
	}

	return extractClaims(mapClaims), nil
}

// ---------- cache logic ----------

func (v *JWKSVerifier) getKey(ctx context.Context, kid string) (interface{}, error) {
	v.mu.RLock()
	if v.keys != nil && time.Since(v.cachedAt) < v.cacheTTL {
		if key, ok := v.keys[kid]; ok {
			v.mu.RUnlock()
			return key, nil
		}
	}
	v.mu.RUnlock()

	// cache miss or stale → refresh
	keys, err := v.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}

	parsed := make(map[string]interface{})
	for _, k := range keys {
		if id := strings.TrimSpace(k.Kid); id != "" {
			if key, err := parseJWK(k); err == nil {
				parsed[id] = key
			}
		}
	}

	v.mu.Lock()
	v.keys = parsed
	v.cachedAt = time.Now()
	v.mu.Unlock()

	if key, ok := parsed[kid]; ok {
		return key, nil
	}

	return nil, fmt.Errorf("kid %q not found", kid)
}

func (v *JWKSVerifier) fetchJWKS(ctx context.Context) ([]jwkKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("JWKS fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("JWKS HTTP %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}
	return jwks.Keys, nil
}

// ---------- JWK parsing ----------

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Crv string `json:"crv"`
	N   string `json:"n"`
	E   string `json:"e"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func parseJWK(k jwkKey) (interface{}, error) {
	switch strings.ToUpper(k.Kty) {
	case "RSA":
		return rsaFromNE(k.N, k.E)
	case "EC":
		return ecFromXY(k.Crv, k.X, k.Y)
	default:
		return nil, fmt.Errorf("unsupported key type %s", k.Kty)
	}
}

func ecFromXY(crv, xB64, yB64 string) (*ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	switch crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported curve %s", crv)
	}

	x, err := decodeB64(xB64)
	if err != nil {
		return nil, err
	}
	y, err := decodeB64(yB64)
	if err != nil {
		return nil, err
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}, nil
}

func rsaFromNE(nB64, eB64 string) (*rsa.PublicKey, error) {
	n, err := decodeB64(nB64)
	if err != nil {
		return nil, err
	}
	e, err := decodeB64(eB64)
	if err != nil {
		return nil, err
	}

	eInt := 0
	for _, b := range e {
		eInt = eInt<<8 + int(b)
	}
	if eInt == 0 {
		return nil, errors.New("invalid exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(n),
		E: eInt,
	}, nil
}

func decodeB64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty base64")
	}
	return base64.RawURLEncoding.DecodeString(s)
}

// ---------- claims ----------

func extractClaims(c jwt.MapClaims) *TokenClaims {
	tc := &TokenClaims{
		Sub:           claimStr(c, "sub"),
		Email:         claimStr(c, "email"),
		EmailVerified: claimBool(c, "email_verified"),
	}

	// Parse exp claim into a concrete time.Time
	if exp, ok := c["exp"]; ok {
		switch v := exp.(type) {
		case float64:
			tc.ExpiresAt = time.Unix(int64(v), 0)
		case json.Number:
			if i, err := v.Int64(); err == nil {
				tc.ExpiresAt = time.Unix(i, 0)
			}
		}
	}

	// Set AuthUserID to sub (Supabase auth_user_id)
	tc.AuthUserID = tc.Sub

	if meta := firstMeta(c); meta != nil {
		// Get internal Threadify user ID from metadata
		tc.UserID = claimStr(meta, "threadify_user_id")
		tc.CompanyID = claimStr(meta, "threadify_company_id")

		if !tc.EmailVerified {
			tc.UserID = claimStr(meta, "threadify_user_id")
			tc.CompanyID = claimStr(meta, "threadify_company_id")
		}
	}

	if r := claimStr(c, "role"); r != "" {
		tc.Roles = []string{r}
	}

	return tc
}

func firstMeta(c jwt.MapClaims) map[string]interface{} {
	for _, key := range []string{"user_metadata", "app_metadata", "metadata"} {
		if m, ok := c[key].(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

func claimStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func claimStrSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		switch arr := v.(type) {
		case []string:
			return filterEmpty(arr)
		case []interface{}:
			out := []string{}
			for _, i := range arr {
				if s, ok := i.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			return out
		case string:
			if s := strings.TrimSpace(arr); s != "" {
				return []string{s}
			}
		}
	}
	return nil
}

func audienceContains(aud interface{}, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}

	switch v := aud.(type) {
	case string:
		return strings.TrimSpace(v) == expected
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) == expected {
				return true
			}
		}
	case []interface{}:
		for _, i := range v {
			if s, ok := i.(string); ok && strings.TrimSpace(s) == expected {
				return true
			}
		}
	}
	return false
}

func filterEmpty(ss []string) []string {
	out := []string{}
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func claimBool(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}

	switch val := v.(type) {
	case bool:
		return val

	case string:
		return strings.EqualFold(strings.TrimSpace(val), "true")
	}

	return false
}
