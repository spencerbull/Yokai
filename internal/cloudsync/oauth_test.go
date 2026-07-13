package cloudsync

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOAuthCallbackIgnoresInvalidState(t *testing.T) {
	t.Parallel()
	results := make(chan oauthCallbackResult, 1)
	handler := oauthCallbackHandler("expected-state", func(result oauthCallbackResult) {
		results <- result
	})

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/oauth/callback?state=wrong&code=attacker", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid state status = %d", invalid.Code)
	}
	select {
	case result := <-results:
		t.Fatalf("invalid state completed callback: %#v", result)
	default:
	}

	valid := httptest.NewRecorder()
	handler.ServeHTTP(valid, httptest.NewRequest(http.MethodGet, "/oauth/callback?state=expected-state&code=valid-code", nil))
	if valid.Code != http.StatusOK {
		t.Fatalf("valid state status = %d", valid.Code)
	}
	if body := valid.Body.String(); !strings.Contains(body, "Google authorization received") || !strings.Contains(body, "Return to Yokai") {
		t.Fatalf("valid callback body = %q", body)
	}
	select {
	case result := <-results:
		if result.code != "valid-code" || result.err != nil {
			t.Fatalf("valid callback result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("valid callback did not complete")
	}
}

func TestRefreshFirebaseIDToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	credentials := &Credentials{
		ProjectID: "yokai-test",
		APIKey:    "public-api-key",
		UID:       "user-123",
		Token: StoredToken{
			IDToken:      "expired-token",
			RefreshToken: "old-refresh-token",
			Expiry:       time.Now().Add(-time.Hour),
		},
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || !strings.Contains(request.URL.String(), "securetoken.googleapis.com/v1/token?key=public-api-key") {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "refresh_token=old-refresh-token") {
			t.Errorf("body = %q", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
              "id_token":"new-id-token",
              "refresh_token":"new-refresh-token",
              "expires_in":"3600",
              "user_id":"user-123"
            }`)),
		}, nil
	})}

	got, err := refreshFirebaseIDToken(context.Background(), credentials, client)
	if err != nil {
		t.Fatalf("refreshFirebaseIDToken() error = %v", err)
	}
	if got != "new-id-token" || credentials.Token.RefreshToken != "new-refresh-token" {
		t.Fatalf("refreshed credentials = %#v", credentials.Token)
	}
	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token.IDToken != "new-id-token" {
		t.Fatal("refreshed token was not persisted")
	}
}

func TestRefreshFirebaseIDTokenReusesFreshToken(t *testing.T) {
	t.Parallel()
	credentials := &Credentials{Token: StoredToken{IDToken: "fresh-token", Expiry: time.Now().Add(time.Hour)}}
	got, err := refreshFirebaseIDToken(context.Background(), credentials, &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("fresh token should not make a request")
		return nil, nil
	})})
	if err != nil || got != "fresh-token" {
		t.Fatalf("refreshFirebaseIDToken() = %q, %v", got, err)
	}
}

func TestLoginBlockingBrowserOpenerHonorsContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	announced := make(chan struct{}, 1)
	started := time.Now()
	_, err := Login(ctx, LoginOptions{
		ProjectID:          "yokai-test",
		APIKey:             "public-api-key",
		GoogleClientID:     "client-id",
		GoogleClientSecret: "client-secret",
		OpenURL: func(string) error {
			<-release
			return nil
		},
		AnnounceURL: func(string) { announced <- struct{}{} },
	})
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("Login() error = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("blocking opener held login for %s", time.Since(started))
	}
	select {
	case <-announced:
	default:
		t.Fatal("fallback URL was not announced")
	}
}
