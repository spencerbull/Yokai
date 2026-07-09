package cloudsync

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
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
