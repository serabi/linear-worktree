package main

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "linear-worktree"
	keyringUser    = "api-key"
)

func storeAPIKey(apiKey string) error {
	return keyring.Set(keyringService, keyringUser, apiKey)
}

func retrieveAPIKey() (string, error) {
	return keyring.Get(keyringService, keyringUser)
}

func deleteAPIKey() error {
	return keyring.Delete(keyringService, keyringUser)
}

func isKeyringNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}
