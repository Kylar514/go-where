package main

import "testing"

func TestAgentRenameUpdatesLocalReferences(t *testing.T) {
	c := defaultConfig()
	c.SSHAgents = map[string]SSHAgentProfile{"old": {SocketEnv: "SSH_AUTH_SOCK"}}
	c.Identities = map[string]LocalIdentity{"work": {SSHAgent: "old"}}
	c.LocalProfiles = map[string]Connection{"p": {ID: "p", ProfileName: "Profile", Port: 22, SSHAgent: "old"}}
	uses, err := agentReferences(c, "old", "new")
	if err != nil || len(uses) != 2 {
		t.Fatal(uses, err)
	}
	if c.Identities["work"].SSHAgent != "new" || c.LocalProfiles["p"].SSHAgent != "new" {
		t.Fatal("reference not renamed")
	}
}
