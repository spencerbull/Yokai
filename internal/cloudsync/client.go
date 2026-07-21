package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseSize = 2 << 20

// ErrNoCloudConfig indicates that the signed-in user has not saved a cloud copy.
var ErrNoCloudConfig = errors.New("no cloud device backup found")

// ErrCloudConfigChanged indicates that a conditional save lost a race with
// another writer and must be retried from a fresh read.
var ErrCloudConfigChanged = errors.New("cloud device backup changed; rerun 'yokai cloud save' to review the latest backup")

// ErrCloudResponseTooLarge indicates that Firestore returned more data than a
// Yokai cloud-config response is allowed to contain.
var ErrCloudResponseTooLarge = errors.New("cloud device backup response is too large")

// Client calls Firestore's REST API using a Firebase user ID token.
type Client struct {
	documentURL string
	idToken     string
	http        *http.Client
}

// Metadata describes a stored cloud snapshot.
type Metadata struct {
	UpdatedAt time.Time `json:"updated_at"`
	Size      int       `json:"size"`
}

type firestoreValue struct {
	IntegerValue string `json:"integerValue,omitempty"`
	StringValue  string `json:"stringValue,omitempty"`
}

type firestoreDocument struct {
	Fields     map[string]firestoreValue `json:"fields"`
	UpdateTime time.Time                 `json:"updateTime"`
}

// NewClient loads and refreshes the local Firebase login.
func NewClient(ctx context.Context) (*Client, *Credentials, error) {
	credentials, err := LoadCredentials()
	if err != nil {
		return nil, nil, err
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	idToken, err := refreshFirebaseIDToken(ctx, credentials, httpClient)
	if err != nil {
		return nil, nil, err
	}
	documentURL := "https://firestore.googleapis.com/v1/projects/" + url.PathEscape(credentials.ProjectID) +
		"/databases/(default)/documents/device_configs/" + url.PathEscape(credentials.UID)
	return &Client{documentURL: documentURL, idToken: idToken, http: httpClient}, credentials, nil
}

// Put uploads one encrypted device configuration envelope. A nil previous
// value requires that no backup exists; a non-nil value requires the exact
// update time observed by Get so concurrent saves cannot silently overwrite.
func (c *Client) Put(ctx context.Context, envelope Envelope, previous *Metadata) (Metadata, error) {
	if err := validateEnvelope(envelope); err != nil {
		return Metadata{}, fmt.Errorf("refusing invalid cloud device config: %w", err)
	}
	target, err := url.Parse(c.documentURL)
	if err != nil {
		return Metadata{}, fmt.Errorf("parsing Firestore document URL: %w", err)
	}
	query := target.Query()
	if previous == nil {
		query.Set("currentDocument.exists", "false")
	} else {
		if previous.UpdatedAt.IsZero() {
			return Metadata{}, fmt.Errorf("cloud device backup update time is missing")
		}
		query.Set("currentDocument.updateTime", previous.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	target.RawQuery = query.Encode()

	document := firestoreDocument{Fields: envelopeToFields(envelope)}
	var response firestoreDocument
	if err := c.do(ctx, http.MethodPatch, target.String(), document, &response); err != nil {
		return Metadata{}, err
	}
	return Metadata{UpdatedAt: response.UpdateTime, Size: len(envelope.Ciphertext)}, nil
}

// Get downloads the current encrypted device configuration envelope.
func (c *Client) Get(ctx context.Context) (Envelope, Metadata, error) {
	var document firestoreDocument
	if err := c.do(ctx, http.MethodGet, c.documentURL, nil, &document); err != nil {
		return Envelope{}, Metadata{}, err
	}
	envelope, err := fieldsToEnvelope(document.Fields)
	if err != nil {
		return Envelope{}, Metadata{}, err
	}
	return envelope, Metadata{UpdatedAt: document.UpdateTime, Size: len(envelope.Ciphertext)}, nil
}

// Delete permanently removes the user's cloud snapshot.
func (c *Client) Delete(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, c.documentURL, nil, nil)
}

func (c *Client) do(ctx context.Context, method, requestURL string, requestBody, responseBody interface{}) error {
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encoding Firestore request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("creating Firestore request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.idToken)
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling Firestore: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("reading Firestore response: %w", err)
	}
	if len(data) > maxResponseSize {
		return ErrCloudResponseTooLarge
	}
	if resp.StatusCode >= 400 {
		var apiError struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &apiError)
		message := apiError.Error.Message
		switch apiError.Error.Status {
		case "FAILED_PRECONDITION", "ABORTED", "ALREADY_EXISTS":
			return ErrCloudConfigChanged
		}
		if resp.StatusCode == http.StatusNotFound && isMissingFirestoreDocument(message) {
			return fmt.Errorf("%w; run 'yokai cloud save' first", ErrNoCloudConfig)
		}
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("firestore: %s", message)
	}
	if responseBody != nil && len(data) > 0 {
		if err := json.Unmarshal(data, responseBody); err != nil {
			return fmt.Errorf("decoding Firestore response: %w", err)
		}
	}
	return nil
}

func isMissingFirestoreDocument(message string) bool {
	message = strings.ToLower(message)
	if !strings.Contains(message, "document") {
		return false
	}
	return strings.Contains(message, "not found") || strings.Contains(message, "no document") || strings.Contains(message, "document missing")
}

func envelopeToFields(envelope Envelope) map[string]firestoreValue {
	return map[string]firestoreValue{
		"version":    {IntegerValue: strconv.Itoa(envelope.Version)},
		"cipher":     {StringValue: envelope.Cipher},
		"kdf":        {StringValue: envelope.KDF},
		"salt":       {StringValue: envelope.Salt},
		"nonce":      {StringValue: envelope.Nonce},
		"ciphertext": {StringValue: envelope.Ciphertext},
	}
}

func fieldsToEnvelope(fields map[string]firestoreValue) (Envelope, error) {
	if len(fields) != 6 {
		return Envelope{}, fmt.Errorf("cloud device config has an invalid field set")
	}
	version, err := strconv.Atoi(fields["version"].IntegerValue)
	if err != nil {
		return Envelope{}, fmt.Errorf("cloud device config has an invalid version")
	}
	envelope := Envelope{
		Version:    version,
		Cipher:     fields["cipher"].StringValue,
		KDF:        fields["kdf"].StringValue,
		Salt:       fields["salt"].StringValue,
		Nonce:      fields["nonce"].StringValue,
		Ciphertext: fields["ciphertext"].StringValue,
	}
	if err := validateEnvelope(envelope); err != nil {
		return Envelope{}, fmt.Errorf("cloud device config is invalid: %w", err)
	}
	return envelope, nil
}
