package tunnelcore

import (
	"encoding/base64"
	"testing"
)

func TestEndpointConflictsAndLegacyBind(t *testing.T) {
	for _, tc := range []struct {
		a, b     string
		conflict bool
	}{
		{"127.0.0.1:8080", "127.0.0.10:8080", false},
		{"127.0.0.1:8080", "127.0.0.1:8080", true},
		{"127.0.0.1:8080", "0.0.0.0:8080", true},
		{"127.0.0.1:8080", "*:8080", true},
		{"127.0.0.1:8080", "[::]:8080", true},
		{"127.0.0.1:8080", "127.0.0.1:8081", false},
		{"[::1]:8080", "127.0.0.1:8080", false},
	} {
		if got := endpointsConflict(tc.a, tc.b); got != tc.conflict {
			t.Fatal(tc, got)
		}
	}
	c := Connection{LocalPort: 8080, Resource: base64.RawURLEncoding.EncodeToString([]byte(`{"Argv":["ssh","-L","127.0.0.10:8080:localhost:80"]}`))}
	if got := connectionEndpoint(c); got != "127.0.0.10:8080" {
		t.Fatal("legacy bind lost", got)
	}
}
