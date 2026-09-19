package accountconnect

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

const testKeyID = "test-key"

type jwtToken struct {
	header    string
	payload   string
	signature string
}

func (t jwtToken) String() string {
	return t.header + "." + t.payload + "." + t.signature
}

func (t jwtToken) hashedContent() []byte {
	sum := sha256.Sum256([]byte(t.header + "." + t.payload))
	return sum[:]
}

type idClaims struct {
	Issuer        string `json:"iss"`
	Audience      string `json:"aud"`
	Expires       int64  `json:"exp"`
	IssuedAt      int64  `json:"iat"`
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
}

func signRS256(t *testing.T, claims idClaims, key *rsa.PrivateKey, alg string) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": alg, "typ": "JWT", "kid": testKeyID})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}

	tok := jwtToken{
		header:  base64.RawURLEncoding.EncodeToString(header),
		payload: base64.RawURLEncoding.EncodeToString(payload),
	}
	if alg == "none" {
		return tok.header + "." + tok.payload + "."
	}

	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, tok.hashedContent())
	if err != nil {
		t.Fatal(err)
	}
	tok.signature = base64.RawURLEncoding.EncodeToString(sig)

	return tok.String()
}

func certClient(t *testing.T, pub rsa.PublicKey) *http.Client {
	t.Helper()

	return &http.Client{Transport: roundTripFn(func(req *http.Request) *http.Response {
		body, err := json.Marshal(map[string]any{
			"keys": []map[string]string{{
				"kid": testKeyID,
				"kty": "RSA",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}
	})}
}

type roundTripFn func(*http.Request) *http.Response

func (f roundTripFn) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func testValidator(t *testing.T, key *rsa.PrivateKey) func(context.Context, string, string) (*idtoken.Payload, error) {
	t.Helper()

	v, err := idtoken.NewValidator(context.Background(), option.WithHTTPClient(certClient(t, key.PublicKey)), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}

	return v.Validate
}

func TestVerifyIDTokenRejectsBadClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	validate := testValidator(t, key)
	audience := "client-123"
	now := time.Now().Add(time.Hour).Unix()
	base := idClaims{
		Issuer:        googleIssuerHTTPS,
		Audience:      audience,
		Expires:       now,
		IssuedAt:      time.Now().Unix(),
		Subject:       "sub-1",
		Email:         "me@gmail.com",
		EmailVerified: true,
	}

	good := signRS256(t, base, key, "RS256")

	payload, err := validate(context.Background(), good, audience)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}

	subject, email, err := identityFromPayload(payload)
	if err != nil || subject != "sub-1" || email != "me@gmail.com" {
		t.Fatalf("identity %s %s %v", subject, email, err)
	}

	unsigned := signRS256(t, base, key, "none")
	if _, valErr := validate(context.Background(), unsigned, audience); valErr == nil {
		t.Fatal("unsigned token accepted")
	}

	wrongAud := base

	wrongAud.Audience = "other-client"
	if _, valErr := validate(context.Background(), signRS256(t, wrongAud, key, "RS256"), audience); valErr == nil {
		t.Fatal("wrong audience accepted")
	}

	expired := base

	expired.Expires = time.Now().Add(-time.Hour).Unix()
	if _, valErr := validate(context.Background(), signRS256(t, expired, key, "RS256"), audience); valErr == nil {
		t.Fatal("expired token accepted")
	}

	wrongIss := base
	wrongIss.Issuer = "https://evil.example"

	payload, err = validate(context.Background(), signRS256(t, wrongIss, key, "RS256"), audience)
	if err != nil {
		t.Fatalf("signature for wrong issuer: %v", err)
	}

	if _, _, idErr := identityFromPayload(payload); idErr == nil {
		t.Fatal("wrong issuer accepted")
	}

	unverified := base
	unverified.EmailVerified = false

	payload, err = validate(context.Background(), signRS256(t, unverified, key, "RS256"), audience)
	if err != nil {
		t.Fatalf("signature for unverified email: %v", err)
	}

	if _, _, idErr := identityFromPayload(payload); idErr == nil {
		t.Fatal("unverified email accepted")
	}
}

func TestGoogleProviderExchangeValidatesIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	claims := idClaims{
		Issuer:        googleIssuerHTTPS,
		Audience:      "client-123",
		Expires:       time.Now().Add(time.Hour).Unix(),
		IssuedAt:      time.Now().Unix(),
		Subject:       "sub-1",
		Email:         "Me@Gmail.com",
		EmailVerified: true,
	}
	idTok := signRS256(t, claims, key, "RS256")
	tokenJSON, _ := json.Marshal(map[string]any{
		"access_token":  "at",
		"token_type":    "Bearer",
		"refresh_token": "rt",
		"id_token":      idTok,
		"scope":         "openid email",
	})
	client := &http.Client{Transport: roundTripFn(func(req *http.Request) *http.Response {
		if req.URL.Path == "/token" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(tokenJSON)), Header: make(http.Header)}
		}
		body, _ := json.Marshal(map[string]any{"keys": []map[string]string{{
			"kid": testKeyID, "kty": "RSA", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})

		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}
	})}
	provider := &GoogleProvider{
		Endpoint:        oauth2.Endpoint{AuthURL: "https://example.test/auth", TokenURL: "https://example.test/token"},
		HTTP:            client,
		ValidateIDToken: testValidator(t, key),
	}

	got, err := provider.Exchange(context.Background(), ExchangeParams{
		ClientID: "client-123", ClientSecret: "secret", RedirectURL: "http://127.0.0.1:9/oauth/callback", Code: "code", Verifier: "verifier",
	})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	if got.Subject != "sub-1" || got.Email != "me@gmail.com" || got.RefreshToken != "rt" {
		t.Fatalf("%+v", got)
	}
}
