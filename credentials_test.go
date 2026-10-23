package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
)

func TestCredentialEncryptionTamperIdentityAndNonce(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	p, err := encryptIdentityPassword(key, "key", "identity", "secret")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decryptIdentityPassword(key, "identity", p)
	if err != nil || plain != "secret" {
		t.Fatal(err)
	}
	if _, err = decryptIdentityPassword(key, "other", p); err == nil {
		t.Fatal("swapped identity accepted")
	}
	q, err := encryptIdentityPassword(key, "key", "identity", "secret")
	if err != nil || bytes.Equal(p.Nonce, q.Nonce) {
		t.Fatal("nonce reused")
	}
	p.Ciphertext[0] ^= 1
	if _, err = decryptIdentityPassword(key, "identity", p); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestCredentialKeyCreatedOnceAndNeverReplaced(t *testing.T) {
	get, set := credentialKeyGet, credentialKeySet
	defer func() { credentialKeyGet, credentialKeySet = get, set }()
	keys := map[string]string{}
	writes := 0
	credentialKeySet = func(service, id, value string) error { writes++; keys[id] = value; return nil }
	credentialKeyGet = func(service, id string) (string, error) {
		v, ok := keys[id]
		if !ok {
			return "", fmt.Errorf("missing")
		}
		return v, nil
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	if _, err := loadConfig(path); err != nil {
		t.Fatal(err)
	}
	p, err := saveIdentityPassword(path, "identity", "secret")
	if err != nil {
		t.Fatal(err)
	}
	q, err := saveIdentityPassword(path, "other", "another")
	if err != nil || p.KeyID != q.KeyID || writes != 1 {
		t.Fatal("key regenerated", err, writes)
	}
	delete(keys, p.KeyID)
	if _, err = saveIdentityPassword(path, "identity", "replacement"); err == nil || writes != 1 {
		t.Fatal("missing key replaced", err, writes)
	}
	data, err := readSettingsData(path)
	if err != nil || bytes.Contains(data, []byte("secret")) {
		t.Fatal("plaintext stored", err)
	}
}
