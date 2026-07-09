package cloudsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientPutGetDelete(t *testing.T) {
	t.Parallel()
	want := Envelope{Version: 1, Cipher: "aes-256-gcm", KDF: "argon2id-3-65536-4", Salt: "salt", Nonce: "nonce", Ciphertext: "ciphertext"}
	updatedAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer firebase-id-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.Method {
		case http.MethodPatch:
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
	metadata, err := client.Put(context.Background(), want)
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

func TestFieldsToEnvelopeRejectsIncompleteDocument(t *testing.T) {
	t.Parallel()
	if _, err := fieldsToEnvelope(map[string]firestoreValue{}); err == nil {
		t.Fatal("fieldsToEnvelope() accepted an incomplete document")
	}
}
