package cloudsync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const firebaseAuthTimeout = 30 * time.Second

// LoginOptions configures Google OAuth and Firebase Authentication.
type LoginOptions struct {
	ProjectID          string
	APIKey             string
	GoogleClientID     string
	GoogleClientSecret string
	OpenURL            func(string) error
	AnnounceURL        func(string)
	HTTPClient         *http.Client
}

// Login signs in with Google using the desktop loopback flow and exchanges the
// Google ID token for a narrow Firebase session used only by this project.
func Login(ctx context.Context, options LoginOptions) (*Credentials, error) {
	if err := validateLoginOptions(options); err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting OAuth callback listener: %w", err)
	}
	defer func() { _ = listener.Close() }()

	redirectURL := "http://" + listener.Addr().String() + "/oauth/callback"
	conf := oauthConfig(options.GoogleClientID, options.GoogleClientSecret, redirectURL)
	state, err := randomURLToken(32)
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()
	authURL := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "select_account"))

	type callbackResult struct {
		code string
		err  error
	}
	callback := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "invalid OAuth state", http.StatusBadRequest)
			callback <- callbackResult{err: fmt.Errorf("OAuth callback state did not match")}
			return
		}
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			http.Error(w, "Google login was not completed", http.StatusBadRequest)
			callback <- callbackResult{err: fmt.Errorf("google login failed: %s", oauthErr)}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			callback <- callbackResult{err: fmt.Errorf("OAuth callback did not include a code")}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>Yokai login complete</title><p>Yokai is connected. You can close this window.</p>"))
		callback <- callbackResult{code: code}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if options.AnnounceURL != nil {
		options.AnnounceURL(authURL)
	}
	opener := options.OpenURL
	if opener == nil {
		opener = openBrowser
	}
	if err := opener(authURL); err != nil && options.AnnounceURL == nil {
		return nil, fmt.Errorf("opening browser: %w", err)
	}

	var result callbackResult
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for Google login: %w", ctx.Err())
	case result = <-callback:
	}
	if result.err != nil {
		return nil, result.err
	}

	googleToken, err := conf.Exchange(ctx, result.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging Google authorization code: %w", err)
	}
	googleIDToken, _ := googleToken.Extra("id_token").(string)
	if googleIDToken == "" {
		return nil, fmt.Errorf("google did not return an ID token")
	}

	firebaseToken, err := exchangeFirebaseToken(ctx, options, googleIDToken)
	if err != nil {
		return nil, err
	}
	credentials := &Credentials{
		ProjectID: options.ProjectID,
		APIKey:    options.APIKey,
		UID:       firebaseToken.LocalID,
		Email:     firebaseToken.Email,
		Token: StoredToken{
			IDToken:      firebaseToken.IDToken,
			RefreshToken: firebaseToken.RefreshToken,
			Expiry:       time.Now().Add(firebaseToken.expiresIn()),
		},
	}
	if err := SaveCredentials(credentials); err != nil {
		return nil, err
	}
	return credentials, nil
}

type firebaseSignInResponse struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
	LocalID      string `json:"localId"`
	Email        string `json:"email"`
}

func (r firebaseSignInResponse) expiresIn() time.Duration {
	seconds, err := strconv.Atoi(r.ExpiresIn)
	if err != nil || seconds < 1 {
		return time.Hour
	}
	return time.Duration(seconds) * time.Second
}

func exchangeFirebaseToken(ctx context.Context, options LoginOptions, googleIDToken string) (firebaseSignInResponse, error) {
	payload := map[string]interface{}{
		"postBody":            "id_token=" + url.QueryEscape(googleIDToken) + "&providerId=google.com",
		"requestUri":          "https://" + options.ProjectID + ".firebaseapp.com/__/auth/handler",
		"returnIdpCredential": true,
		"returnSecureToken":   true,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return firebaseSignInResponse{}, fmt.Errorf("encoding Firebase sign-in: %w", err)
	}
	requestURL := "https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=" + url.QueryEscape(options.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(data))
	if err != nil {
		return firebaseSignInResponse{}, fmt.Errorf("creating Firebase sign-in request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: firebaseAuthTimeout}
	}
	var response firebaseSignInResponse
	if err := doFirebaseAuthRequest(client, req, &response); err != nil {
		return firebaseSignInResponse{}, err
	}
	if response.IDToken == "" || response.RefreshToken == "" || response.LocalID == "" {
		return firebaseSignInResponse{}, fmt.Errorf("firebase sign-in response was incomplete")
	}
	return response, nil
}

func refreshFirebaseIDToken(ctx context.Context, credentials *Credentials, client *http.Client) (string, error) {
	if credentials.Token.IDToken != "" && credentials.Token.Expiry.After(time.Now().Add(5*time.Minute)) {
		return credentials.Token.IDToken, nil
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {credentials.Token.RefreshToken},
	}
	requestURL := "https://securetoken.googleapis.com/v1/token?key=" + url.QueryEscape(credentials.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating Firebase refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if client == nil {
		client = &http.Client{Timeout: firebaseAuthTimeout}
	}
	var response struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
		UserID       string `json:"user_id"`
	}
	if err := doFirebaseAuthRequest(client, req, &response); err != nil {
		return "", fmt.Errorf("refreshing Firebase login: %w", err)
	}
	if response.IDToken == "" || response.RefreshToken == "" || response.UserID != credentials.UID {
		return "", fmt.Errorf("firebase refresh response was incomplete or changed user")
	}
	seconds, _ := strconv.Atoi(response.ExpiresIn)
	if seconds < 1 {
		seconds = 3600
	}
	credentials.Token = StoredToken{
		IDToken:      response.IDToken,
		RefreshToken: response.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(seconds) * time.Second),
	}
	if err := SaveCredentials(credentials); err != nil {
		return "", err
	}
	return response.IDToken, nil
}

func doFirebaseAuthRequest(client *http.Client, req *http.Request, result interface{}) error {
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling Firebase Authentication: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading Firebase Authentication response: %w", err)
	}
	if resp.StatusCode >= 400 {
		var apiError struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &apiError)
		message := apiError.Error.Message
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("firebase authentication: %s", message)
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decoding Firebase Authentication response: %w", err)
	}
	return nil
}

func oauthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}
}

func validateLoginOptions(options LoginOptions) error {
	if strings.TrimSpace(options.ProjectID) == "" || strings.TrimSpace(options.APIKey) == "" {
		return fmt.Errorf("firebase project ID and API key are required")
	}
	if strings.ContainsAny(options.ProjectID, "/?#") {
		return fmt.Errorf("firebase project ID is invalid")
	}
	if strings.TrimSpace(options.GoogleClientID) == "" || strings.TrimSpace(options.GoogleClientSecret) == "" {
		return fmt.Errorf("google OAuth client ID and client secret are required")
	}
	return nil
}

func randomURLToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(targetURL string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{targetURL}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", targetURL}
	default:
		command = "xdg-open"
		args = []string{targetURL}
	}
	return exec.Command(command, args...).Start()
}
