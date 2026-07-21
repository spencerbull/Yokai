package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

func clientTestEnvelope(t *testing.T, deviceID string) Envelope {
	t.Helper()
	envelope, err := Encrypt(Snapshot{
		Version: SnapshotVersion,
		Devices: []config.Device{{ID: deviceID}},
	}, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestClientPutGetDelete(t *testing.T) {
	t.Parallel()
	want := clientTestEnvelope(t, "device-a")
	updatedAt := time.Date(2026, 7, 9, 12, 0, 0, 123000000, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer firebase-id-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.Method {
		case http.MethodPatch:
			if got := r.URL.Query().Get("currentDocument.exists"); got != "false" {
				t.Errorf("currentDocument.exists = %q", got)
			}
			var document firestoreDocument
			if err := json.NewDecoder(r.Body).Decode(&document); err != nil {
				t.Error(err)
				return
			}
			got, err := fieldsToEnvelope(document.Fields)
			if err != nil || got != want {
				t.Errorf("PATCH body = %#v, %v", got, err)
			}
			_ = json.NewEncoder(w).Encode(firestoreDocument{Fields: document.Fields, UpdateTime: updatedAt})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(firestoreDocument{Fields: envelopeToFields(want), UpdateTime: updatedAt})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := &Client{documentURL: server.URL + "/config", idToken: "firebase-id-token", http: server.Client()}
	metadata, err := client.Put(context.Background(), want, nil)
	if err != nil || !metadata.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("Put() = %#v, %v", metadata, err)
	}
	got, metadata, err := client.Get(context.Background())
	if err != nil || got != want || !metadata.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("Get() = %#v, %#v, %v", got, metadata, err)
	}
	if err := client.Delete(context.Background()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestClientConditionalPutPreventsLostUpdatesAndCreateRaces(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	exists := true
	stored := clientTestEnvelope(t, "original")
	updatedAt := time.Date(2026, 7, 9, 12, 0, 0, 123000000, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			if !exists {
				writeFirestoreError(w, http.StatusNotFound, "NOT_FOUND", "Document device_configs/test-user not found.")
				return
			}
			_ = json.NewEncoder(w).Encode(firestoreDocument{Fields: envelopeToFields(stored), UpdateTime: updatedAt})
		case http.MethodPatch:
			query := r.URL.Query()
			if query.Get("currentDocument.exists") == "false" {
				if exists {
					writeFirestoreError(w, http.StatusConflict, "ALREADY_EXISTS", "document already exists")
					return
				}
			} else if query.Get("currentDocument.updateTime") != updatedAt.Format(time.RFC3339Nano) || !exists {
				writeFirestoreError(w, http.StatusBadRequest, "FAILED_PRECONDITION", "document update time changed")
				return
			}

			var document firestoreDocument
			if err := json.NewDecoder(r.Body).Decode(&document); err != nil {
				t.Error(err)
				return
			}
			next, err := fieldsToEnvelope(document.Fields)
			if err != nil {
				t.Error(err)
				return
			}
			exists = true
			stored = next
			updatedAt = updatedAt.Add(time.Second)
			_ = json.NewEncoder(w).Encode(firestoreDocument{Fields: document.Fields, UpdateTime: updatedAt})
		}
	}))
	defer server.Close()

	first := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
	second := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
	_, firstMetadata, err := first.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, secondMetadata, err := second.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	firstReplacement := clientTestEnvelope(t, "first-writer")
	if _, err := first.Put(context.Background(), firstReplacement, &firstMetadata); err != nil {
		t.Fatalf("first replacement error = %v", err)
	}
	secondReplacement := clientTestEnvelope(t, "stale-writer")
	if _, err := second.Put(context.Background(), secondReplacement, &secondMetadata); !errors.Is(err, ErrCloudConfigChanged) {
		t.Fatalf("stale replacement error = %v", err)
	}
	got, _, err := first.Get(context.Background())
	if err != nil || got != firstReplacement {
		t.Fatalf("stored after stale replacement = %#v, %v", got, err)
	}

	mu.Lock()
	exists = false
	mu.Unlock()
	firstCreate := clientTestEnvelope(t, "first-create")
	if _, err := first.Put(context.Background(), firstCreate, nil); err != nil {
		t.Fatalf("first create error = %v", err)
	}
	if _, err := second.Put(context.Background(), clientTestEnvelope(t, "second-create"), nil); !errors.Is(err, ErrCloudConfigChanged) {
		t.Fatalf("second create error = %v", err)
	}
	got, _, err = first.Get(context.Background())
	if err != nil || got != firstCreate {
		t.Fatalf("stored after create race = %#v, %v", got, err)
	}
}

func writeFirestoreError(w http.ResponseWriter, statusCode int, status, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"status": status, "message": message},
	})
}

func TestClientPutRejectsInvalidEnvelopeBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}

	invalid := clientTestEnvelope(t, "invalid")
	invalid.Nonce = "short"
	if _, err := client.Put(context.Background(), invalid, nil); err == nil {
		t.Fatal("Put() accepted an invalid envelope")
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid Put() sent %d request(s)", requests.Load())
	}
	if _, err := client.Put(context.Background(), clientTestEnvelope(t, "missing-time"), &Metadata{}); err == nil {
		t.Fatal("Put() accepted an empty update time")
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid conditional Put() sent %d request(s)", requests.Load())
	}
}

func TestFieldsToEnvelopeRejectsInvalidDocuments(t *testing.T) {
	t.Parallel()
	valid := envelopeToFields(clientTestEnvelope(t, "valid"))
	tests := []struct {
		name   string
		mutate func(map[string]firestoreValue)
	}{
		{name: "missing field", mutate: func(fields map[string]firestoreValue) { delete(fields, "salt") }},
		{name: "extra field", mutate: func(fields map[string]firestoreValue) {
			fields["email"] = firestoreValue{StringValue: "leak@example.com"}
		}},
		{name: "wrong version type", mutate: func(fields map[string]firestoreValue) { fields["version"] = firestoreValue{StringValue: "1"} }},
		{name: "wrong string type", mutate: func(fields map[string]firestoreValue) { fields["nonce"] = firestoreValue{IntegerValue: "12"} }},
		{name: "invalid encoding", mutate: func(fields map[string]firestoreValue) { fields["salt"] = firestoreValue{StringValue: "not-base64!"} }},
		{name: "oversized ciphertext", mutate: func(fields map[string]firestoreValue) {
			fields["ciphertext"] = firestoreValue{StringValue: strings.Repeat("A", maxEncodedCiphertextSize+1)}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := make(map[string]firestoreValue, len(valid)+1)
			for key, value := range valid {
				fields[key] = value
			}
			test.mutate(fields)
			if _, err := fieldsToEnvelope(fields); err == nil {
				t.Fatal("fieldsToEnvelope() accepted invalid document")
			}
		})
	}
}

func TestClientGetResponseSizeBoundary(t *testing.T) {
	t.Parallel()
	envelope := clientTestEnvelope(t, "boundary")
	document, err := json.Marshal(firestoreDocument{Fields: envelopeToFields(envelope), UpdateTime: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("exact limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(append(document, []byte(strings.Repeat(" ", maxResponseSize-len(document)))...))
		}))
		defer server.Close()
		client := &Client{documentURL: server.URL, idToken: "token", http: server.Client()}
		if _, _, err := client.Get(context.Background()); err != nil {
			t.Fatalf("exact-limit Get() error = %v", err)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", maxResponseSize+1)))
		}))
		defer server.Close()
		client := &Client{documentURL: server.URL, idToken: "token", http: server.Client()}
		if _, _, err := client.Get(context.Background()); !errors.Is(err, ErrCloudResponseTooLarge) {
			t.Fatalf("over-limit Get() error = %v", err)
		}
	})
}

func TestClientGetMapsMissingBackup(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeFirestoreError(w, http.StatusNotFound, "NOT_FOUND", "Document device_configs/test-user not found.")
	}))
	defer server.Close()

	client := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
	_, _, err := client.Get(context.Background())
	if !errors.Is(err, ErrNoCloudConfig) {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestClientDoesNotMapBackend404ToMissingBackup(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeFirestoreError(w, http.StatusNotFound, "NOT_FOUND", "Project yokai-missing not found.")
	}))
	defer server.Close()

	client := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
	_, _, err := client.Get(context.Background())
	if err == nil || errors.Is(err, ErrNoCloudConfig) || !strings.Contains(err.Error(), "Project yokai-missing") {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestClientConflictMappingIsPatchSpecific(t *testing.T) {
	t.Parallel()

	t.Run("empty precondition response on PATCH", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusPreconditionFailed)
		}))
		defer server.Close()

		client := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
		if _, err := client.Put(context.Background(), clientTestEnvelope(t, "conflict"), nil); !errors.Is(err, ErrCloudConfigChanged) {
			t.Fatalf("Put() error = %v", err)
		}
	})

	t.Run("structured precondition response on GET", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFirestoreError(w, http.StatusBadRequest, "FAILED_PRECONDITION", "backend index unavailable")
		}))
		defer server.Close()

		client := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
		_, _, err := client.Get(context.Background())
		if err == nil || errors.Is(err, ErrCloudConfigChanged) || !strings.Contains(err.Error(), "backend index unavailable") {
			t.Fatalf("Get() error = %v", err)
		}
	})
}

func TestClientRejectsSuccessfulResponsesWithoutUpdateTime(t *testing.T) {
	t.Parallel()
	envelope := clientTestEnvelope(t, "missing-update-time")

	t.Run("Put", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(firestoreDocument{Fields: envelopeToFields(envelope)})
		}))
		defer server.Close()

		client := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
		if _, err := client.Put(context.Background(), envelope, nil); err == nil || !strings.Contains(err.Error(), "update time") {
			t.Fatalf("Put() error = %v", err)
		}
	})

	t.Run("Get", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(firestoreDocument{Fields: envelopeToFields(envelope)})
		}))
		defer server.Close()

		client := &Client{documentURL: server.URL + "/config", idToken: "token", http: server.Client()}
		if _, _, err := client.Get(context.Background()); err == nil || !strings.Contains(err.Error(), "update time") {
			t.Fatalf("Get() error = %v", err)
		}
	})
}
