package main

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func init() {
	// Use mock keyring backend in tests
	keyring.MockInit()
}

func TestStoreAndRetrieveAPIKey(t *testing.T) {
	err := storeAPIKey("lin_api_test123")
	if err != nil {
		t.Fatalf("storeAPIKey() error: %v", err)
	}

	key, err := retrieveAPIKey()
	if err != nil {
		t.Fatalf("retrieveAPIKey() error: %v", err)
	}
	if key != "lin_api_test123" {
		t.Errorf("retrieveAPIKey() = %q, want %q", key, "lin_api_test123")
	}
}

func TestDeleteAPIKey(t *testing.T) {
	storeAPIKey("lin_api_todelete")

	err := deleteAPIKey()
	if err != nil {
		t.Fatalf("deleteAPIKey() error: %v", err)
	}

	_, err = retrieveAPIKey()
	if !isKeyringNotFound(err) {
		t.Errorf("expected not-found error after delete, got: %v", err)
	}
}

func TestRetrieveAPIKeyNotFound(t *testing.T) {
	// Clean slate — delete in case a previous test left something
	deleteAPIKey()

	_, err := retrieveAPIKey()
	if !isKeyringNotFound(err) {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestIsKeyringNotFound(t *testing.T) {
	if isKeyringNotFound(nil) {
		t.Error("nil should not be not-found")
	}
	if !isKeyringNotFound(keyring.ErrNotFound) {
		t.Error("ErrNotFound should be not-found")
	}
	if isKeyringNotFound(os.ErrNotExist) {
		t.Error("other errors should not be not-found")
	}
}
