package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type apiUnavailableError struct{ message string }

func (e apiUnavailableError) Error() string { return e.message }

var apiDiskCacheDir = func() (string, error) {
	dir, err := os.UserCacheDir()
	return filepath.Join(dir, "go-where", "api-inventory"), err
}

func apiDiskCachePath(s InventorySource) (string, error) {
	dir, err := apiDiskCacheDir()
	if err != nil {
		return "", err
	}
	// Credential scoping prevents a different user/token registration inheriting
	// an earlier user's cached access to the same inventory.
	hash := sha256.Sum256([]byte(apiCacheKey(s)))
	return filepath.Join(dir, hex.EncodeToString(hash[:])+".json"), nil
}

func saveAPIDiskCache(s InventorySource, v apiSnapshot) error {
	path, err := apiDiskCachePath(s)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".snapshot-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replaceAPICache(f.Name(), path)
}

func loadAPIDiskCache(s InventorySource) (apiSnapshot, error) {
	var v apiSnapshot
	path, err := apiDiskCachePath(s)
	if err != nil {
		return v, err
	}
	f, err := os.Open(path)
	if err != nil {
		return v, err
	}
	defer f.Close()
	if err = json.NewDecoder(io.LimitReader(f, apiBodyLimit+1)).Decode(&v); err != nil {
		return v, err
	}
	if v.FetchedAt.IsZero() || v.Revision == "" {
		return v, fmt.Errorf("invalid cached inventory")
	}
	return v, nil
}

func clearAPIDiskCache(s InventorySource) error {
	apiSnapshots.Lock()
	delete(apiSnapshots.values, apiCacheKey(s))
	apiSnapshots.Unlock()
	path, err := apiDiskCachePath(s)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func apiSourceAvailability(s InventorySource) string {
	apiSnapshots.Lock()
	v, ok := apiSnapshots.values[apiCacheKey(s)]
	apiSnapshots.Unlock()
	if ok && v.Offline {
		return "Offline · Cached " + v.FetchedAt.Local().Format("Jan 02, 15:04") + " · Read-only"
	}
	return ""
}
