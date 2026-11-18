package main

import "fmt"

func rotateAPIToken(s InventorySource, replacement serverToken) error {
	old, e := apiTokenGet(apiCredentialService, s.CredentialID)
	if e != nil {
		return e
	}
	if e = apiTokenSet(apiCredentialService, s.CredentialID, replacement.Secret); e != nil {
		_ = apiRequest(s, "DELETE", "tokens/"+replacement.ID, nil, nil, "")
		return e
	}
	var checked []serverToken
	if e = apiRequest(s, "GET", "tokens", nil, &checked, ""); e != nil {
		if restore := apiTokenSet(apiCredentialService, s.CredentialID, old); restore != nil {
			return fmt.Errorf("token verification failed; replacement remains saved and old token was not revoked")
		}
		_ = apiRequest(s, "DELETE", "tokens/"+replacement.ID, nil, nil, "")
		return e
	}
	temp := inventoryID()
	if e = apiTokenSet(apiCredentialService, temp, old); e != nil {
		return fmt.Errorf("replacement saved, old token not revoked: %w", e)
	}
	defer apiTokenDelete(apiCredentialService, temp)
	oldSource := s
	oldSource.CredentialID = temp
	if e = apiRequest(oldSource, "DELETE", "tokens/current", nil, nil, ""); e != nil {
		return fmt.Errorf("replacement token saved and verified, but old-token revocation failed: %w", e)
	}
	return nil
}
