package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const apiCredentialService = "go-where-api"
const apiBodyLimit = 8 << 20

type apiSnapshot struct {
	Hosts       []InventoryHost `json:"hosts"`
	Profiles    []Connection    `json:"profiles"`
	Connections []Connection    `json:"connections"`
	Revision    string          `json:"revision"`
	ReadOnly    bool            `json:"read_only"`
}

type apiMutation struct {
	Action      string            `json:"action"`
	Host        InventoryHost     `json:"host,omitempty"`
	Profiles    []Connection      `json:"profiles,omitempty"`
	Connection  Connection        `json:"connection,omitempty"`
	Remove      bool              `json:"remove,omitempty"`
	ID          string            `json:"id,omitempty"`
	BeforeHosts []InventoryHost   `json:"before_hosts,omitempty"`
	Edits       map[string]string `json:"edits,omitempty"`
	Before      []Connection      `json:"before,omitempty"`
	After       []Connection      `json:"after,omitempty"`
	Old         Connection        `json:"old,omitempty"`
	Replacement Connection        `json:"replacement,omitempty"`
}

var apiHTTPClient = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("API redirects are not allowed") },
}
var apiTokenGet = credentialKeyGet
var apiTokenSet = credentialKeySet
var apiTokenDelete = deleteAPICredential
var apiSnapshots = struct {
	sync.Mutex
	values map[string]apiSnapshot
}{values: map[string]apiSnapshot{}}

func validateAPIURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return fmt.Errorf("API URL must be an HTTPS origin without credentials, query, or path")
	}
	return nil
}

func apiRequest(s InventorySource, method, path string, input, output any, revision string) error {
	if err := validateAPIURL(s.URL); err != nil {
		return err
	}
	token, err := apiTokenGet(apiCredentialService, s.CredentialID)
	if err != nil {
		return fmt.Errorf("API credential unavailable; reconnect this server in Sources")
	}
	var body io.Reader
	if input != nil {
		data, e := json.Marshal(input)
		if e != nil {
			return e
		}
		body = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.URL, "/")+"/v1/"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if revision != "" {
		req.Header.Set("If-Match", revision)
	}
	response, err := apiHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("API unavailable; check server address, TLS trust, and connectivity")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		switch response.StatusCode {
		case 401:
			return fmt.Errorf("API token invalid, expired, revoked, or user disabled")
		case 403:
			return fmt.Errorf("API access denied (read-only or inventory not permitted)")
		case 412:
			return fmt.Errorf("inventory changed since it was loaded; unsaved draft retained, cancel/reload and review before retrying")
		}
		var message struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&message)
		if message.Error != "" {
			return fmt.Errorf("API: %s", message.Error)
		}
		return fmt.Errorf("API request failed (%d)", response.StatusCode)
	}
	if output != nil {
		return json.NewDecoder(io.LimitReader(response.Body, apiBodyLimit+1)).Decode(output)
	}
	return nil
}

func apiCacheKey(s InventorySource) string { return s.URL + "|" + s.CredentialID + "|" + s.ID }

func readAPISnapshot(s InventorySource) (apiSnapshot, error) {
	key := apiCacheKey(s)
	apiSnapshots.Lock()
	v, ok := apiSnapshots.values[key]
	apiSnapshots.Unlock()
	if ok {
		data, _ := json.Marshal(v)
		var copy apiSnapshot
		if err := json.Unmarshal(data, &copy); err != nil {
			return v, err
		}
		profiles := map[string]Connection{}
		for _, p := range copy.Profiles {
			profiles[p.ID] = p
		}
		for i := range copy.Connections {
			if p, ok := profiles[copy.Connections[i].ProfileID]; ok {
				original := p
				copy.Connections[i].OriginalProfile = &original
			}
		}
		return copy, nil
	}
	if err := apiRequest(s, "GET", "inventories/"+s.ID, nil, &v, ""); err != nil {
		return v, err
	}
	if v.Revision == "" {
		return v, fmt.Errorf("API returned inventory without revision")
	}
	if err := validateConnections(v.Connections); err != nil {
		return v, fmt.Errorf("invalid remote inventory: %w", err)
	}
	for _, h := range v.Hosts {
		if !inventoryName(h.ID) || !inventoryName(h.Alias) || !safeValue(h.Address) || !safeValue(h.Notes) {
			return v, fmt.Errorf("invalid remote host")
		}
		if validateHostColor(h.Color) != nil || validateBracketColor(h.BracketColor) != nil {
			return v, fmt.Errorf("invalid remote host colors")
		}
	}
	// Reconstruct optimistic profile baselines omitted from JSON.
	profiles := map[string]Connection{}
	for _, p := range v.Profiles {
		profiles[p.ID] = p
	}
	for i := range v.Connections {
		r := &v.Connections[i]
		if p, ok := profiles[r.ProfileID]; ok {
			original := p
			r.OriginalProfile = &original
		}
	}
	apiSnapshots.Lock()
	apiSnapshots.values[key] = v
	apiSnapshots.Unlock()
	return readAPISnapshot(s)
}

func refreshAPISources(c Config) {
	apiSnapshots.Lock()
	defer apiSnapshots.Unlock()
	for _, s := range c.Sources {
		if s.Type == "api" {
			delete(apiSnapshots.values, apiCacheKey(s))
		}
	}
}

func mutateAPI(s InventorySource, change apiMutation) error {
	if s.ReadOnly {
		return fmt.Errorf("source is read-only")
	}
	v, err := readAPISnapshot(s)
	if err != nil {
		return err
	}
	if v.ReadOnly {
		return fmt.Errorf("source is read-only")
	}
	if err = apiRequest(s, "POST", "inventories/"+s.ID, change, nil, v.Revision); err != nil {
		return err
	}
	apiSnapshots.Lock()
	delete(apiSnapshots.values, apiCacheKey(s))
	apiSnapshots.Unlock()
	return nil
}

func localAPISnapshot(s InventorySource) (apiSnapshot, error) {
	db, err := openInventory(s)
	if err != nil {
		return apiSnapshot{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return apiSnapshot{}, err
	}
	defer tx.Rollback()
	var v apiSnapshot
	if v.Hosts, err = readInventoryHosts(tx); err != nil {
		return v, err
	}
	if v.Profiles, err = readReusableProfiles(tx); err != nil {
		return v, err
	}
	if v.Connections, err = readHostConnections(tx); err != nil {
		return v, err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	sum := sha256.Sum256(data)
	v.Revision = hex.EncodeToString(sum[:])
	return v, nil
}

func applyAPIMutation(s InventorySource, m apiMutation) error {
	if m.Action == "bulk" {
		profiles, err := sourceProfiles(s)
		if err != nil {
			return err
		}
		byID := map[string]Connection{}
		for _, p := range profiles {
			byID[p.ID] = p
		}
		for i := range m.Before {
			if p, ok := byID[m.Before[i].ProfileID]; ok {
				original := p
				m.Before[i].OriginalProfile = &original
			}
		}
	}
	switch m.Action {
	case "host-save":
		return saveInventoryHost(s, m.Host, m.Profiles)
	case "host-fields":
		for field := range m.Edits {
			switch field {
			case "name", "address", "tags", "notes", "color", "bracket_color":
			default:
				return fmt.Errorf("unknown host field")
			}
		}
		return saveHostFields(s, m.BeforeHosts, m.Edits)
	case "host-delete":
		return deleteInventoryHost(s, m.ID)
	case "connection":
		return changeConnection(s, m.Connection, m.Remove)
	case "profile-save":
		return saveReusableProfile(s, m.Connection)
	case "profile-delete":
		return deleteConnectionProfile(Config{}, "", s, m.ID)
	case "bulk":
		if len(m.Before) != len(m.After) {
			return fmt.Errorf("bulk edit lengths differ")
		}
		return applyBulkConnections(s, m.Before, m.After)
	case "profile-replace":
		return replaceInventoryProfile(s, m.Old, m.Replacement)
	default:
		return fmt.Errorf("unknown inventory action")
	}
}
