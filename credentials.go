package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

type EncryptedPassword struct {
	Version    int    `json:"version"`
	KeyID      string `json:"key_id"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

var credentialKeyGet = keyring.Get
var credentialKeySet = keyring.Set

const credentialService = "go-where"

func encryptIdentityPassword(key []byte, keyID, id, password string) (EncryptedPassword, error) {
	if len(key) != 32 || id == "" || password == "" || !safeValue(password) || len(password) > 1024 {
		return EncryptedPassword{}, fmt.Errorf("invalid credential input")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedPassword{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedPassword{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return EncryptedPassword{}, err
	}
	return EncryptedPassword{1, keyID, nonce, gcm.Seal(nil, nonce, []byte(password), []byte("go-where/rdp/"+id))}, nil
}

func decryptIdentityPassword(key []byte, id string, p EncryptedPassword) (string, error) {
	if len(key) != 32 || p.Version != 1 || id == "" {
		return "", fmt.Errorf("unsupported credential record")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(p.Nonce) != gcm.NonceSize() {
		return "", fmt.Errorf("invalid credential nonce")
	}
	plain, err := gcm.Open(nil, p.Nonce, p.Ciphertext, []byte("go-where/rdp/"+id))
	if err != nil {
		return "", fmt.Errorf("credential authentication failed")
	}
	defer clear(plain)
	return string(plain), nil
}

func loadCredentialKey(id string) ([]byte, error) {
	value, err := credentialKeyGet(credentialService, id)
	if err != nil {
		return nil, fmt.Errorf("credential key unavailable: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("invalid stored credential key")
	}
	return key, nil
}

func credentialPassword(path, id string) (string, error) {
	c, err := loadConfig(path)
	if err != nil {
		return "", err
	}
	for _, a := range c.RDP.Accounts {
		if a.ID == id && a.Password != nil {
			key, err := loadCredentialKey(a.Password.KeyID)
			if err != nil {
				return "", err
			}
			defer clear(key)
			return decryptIdentityPassword(key, id, *a.Password)
		}
	}
	return "", fmt.Errorf("saved credential unavailable")
}

func saveIdentityPassword(path, id, password string) (EncryptedPassword, error) {
	if !databaseSettings(path) {
		return EncryptedPassword{}, fmt.Errorf("saved passwords require the local settings database")
	}
	lock, err := os.OpenFile(path+".credentials.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return EncryptedPassword{}, fmt.Errorf("credential save is locked: %w", err)
	}
	lock.Close()
	defer os.Remove(path + ".credentials.lock")
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return EncryptedPassword{}, err
	}
	keyID := draft.CredentialKeyID
	if keyID == "" {
		for _, a := range draft.RDP.Accounts {
			if a.Password != nil {
				return EncryptedPassword{}, fmt.Errorf("encrypted credentials exist without a key identifier; refusing replacement")
			}
		}
		keyID = inventoryID()
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return EncryptedPassword{}, err
		}
		defer clear(key)
		if err = credentialKeySet(credentialService, keyID, base64.StdEncoding.EncodeToString(key)); err != nil {
			return EncryptedPassword{}, fmt.Errorf("store credential key: %w", err)
		}
		draft.CredentialKeyID = keyID
		if err = saveSettingsDraft(path, original, before, draft); err != nil {
			return EncryptedPassword{}, err
		}
	}
	key, err := loadCredentialKey(keyID)
	if err != nil {
		return EncryptedPassword{}, err
	}
	defer clear(key)
	return encryptIdentityPassword(key, keyID, id, password)
}
