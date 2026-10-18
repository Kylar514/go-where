package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRDPEnvironmentMetadata(t *testing.T) {
	r := defaultRDPConfig()
	r.User, r.DefaultDomain = "operator", "ad.example"
	r.Environments = []RDPEnvironment{
		{Name: "PROD", Jump: "prod-*"},
		{Name: "DEV", Jump: "lab-*", Aliases: []string{"*dev*"}},
		{Name: "QA", Jump: "lab-*", Aliases: []string{"*qa*"}},
		{Name: "STG", Jump: "lab-*", Aliases: []string{"*stg*", "*tst*"}},
		{Name: "AZURE", Aliases: []string{"az-*"}},
	}
	if err := r.validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ alias, jump, env string }{
		{"app-dev", "prod-1", "PROD"}, // jump-only rule wins before DEV alias.
		{"APP-DEV", "user@LAB-1:2222,inner", "DEV"},
		{"app-qa", "lab-1", "QA"},
		{"app-stg", "lab-1", "STG"},
		{"app-tst", "lab-1", "STG"},
		{"az-web", "", "AZURE"},
		{"app-dev", "other", ""}, // both jump and alias must match.
		{"other", "lab-1", ""},
		{"app-dev", "", ""},
	} {
		h := Host{Alias: tc.alias, Hostname: "192.0.2.1", JumpHost: tc.jump}
		job, err := newRDPJob(r, h, "")
		if err != nil {
			t.Fatal(err)
		}
		want := tc.alias + ` — ad.example\operator`
		if tc.env != "" {
			want = "[" + tc.env + "] " + want
		}
		if job.Title != want || !hasArg(job.clientArgs("direct", 0), "/title:"+want) {
			t.Fatalf("%s via %s: title %q, want %q", tc.alias, tc.jump, job.Title, want)
		}
		decoded, err := decodeRDPJob(encodeRDPJob(job))
		if err != nil || decoded.Title != want || !decoded.TitleBar {
			t.Fatalf("window metadata lost in detached job: %+v, %v", decoded, err)
		}
	}
	host := Host{Alias: "app-qa", Hostname: "192.0.2.1", JumpHost: "lab-1"}
	domain := "admin.example"
	job, err := newRDPJob(r.withAccount(RDPAccount{Name: "Admin", User: "admin", Domain: &domain}), host, "")
	if err != nil || job.Title != `[QA] app-qa — admin.example\admin` {
		t.Fatalf("title must reflect selected account without changing environment: %+v, %v", job, err)
	}
	r.TitleBar = false
	job, _ = newRDPJob(r, host, "")
	if strings.Contains(strings.Join(job.clientArgs("socks", 41000), " "), "/floatbar") {
		t.Fatal("title_bar false must disable the default floatbar")
	}
	r.ClientArgs = []string{"/title:custom", "/floatbar:sticky:off,default:hidden,show:always"}
	r.TitleBar = true
	job, _ = newRDPJob(r, host, "")
	args := strings.Join(job.clientArgs("direct", 0), " ")
	if strings.Count(args, "/title:") != 1 || strings.Count(args, "/floatbar:") != 1 || !strings.Contains(args, "/title:custom") {
		t.Fatalf("explicit title/bar overrides must win: %s", args)
	}
	for _, invalid := range []RDPEnvironment{
		{Name: "x\n", Jump: "*"},
		{Name: "", Jump: "*"},
		{Name: "DEV", Jump: "["},
		{Name: "DEV", Aliases: []string{""}},
		{Name: "DEV"},
	} {
		if validateEnvironments([]RDPEnvironment{invalid}) == nil {
			t.Fatalf("invalid environment accepted: %+v", invalid)
		}
	}
	if validateEnvironments([]RDPEnvironment{r.Environments[0], r.Environments[0]}) == nil {
		t.Fatal("duplicate environment names accepted")
	}
	config := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, config, `{"rdp":{"environments":[{"name":"QA","jump":"lab-*"}]}}`)
	c, err := loadConfig(config)
	if err != nil || !c.RDP.TitleBar || len(c.RDP.Environments) != 1 {
		t.Fatalf("config/default loading: %+v, %v", c.RDP, err)
	}
	writeTestFile(t, config, `{"rdp":{"environments":[{"name":"QA","aliases":["*qa*"],"color":"#ebb450"}]}}`)
	if _, err := loadConfig(config); err == nil {
		t.Fatal("obsolete compositor color setting must be rejected")
	}
	var roundTrip RDPConfig
	b, _ := json.Marshal(r)
	if err := json.Unmarshal(b, &roundTrip); err != nil || len(roundTrip.Environments) != len(r.Environments) {
		t.Fatal("environment config did not round-trip")
	}
}
