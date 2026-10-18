package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

type RDPEnvironment struct {
	Name    string   `json:"name"`
	Jump    string   `json:"jump,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

var environmentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _-]*$`)

func validateEnvironments(environments []RDPEnvironment) error {
	names := map[string]bool{}
	for _, env := range environments {
		if !environmentName.MatchString(env.Name) || names[strings.ToLower(env.Name)] {
			return fmt.Errorf("rdp environments need unique names using letters, digits, spaces, _ or -")
		}
		names[strings.ToLower(env.Name)] = true
		if env.Jump == "" && len(env.Aliases) == 0 {
			return fmt.Errorf("rdp environment %q needs jump or aliases patterns", env.Name)
		}
		patterns := append([]string{}, env.Aliases...)
		if env.Jump != "" {
			patterns = append(patterns, env.Jump)
		}
		for _, pattern := range patterns {
			if _, err := path.Match(pattern, ""); err != nil || pattern == "" || !safeValue(pattern) {
				return fmt.Errorf("rdp environment %q: invalid pattern %q", env.Name, pattern)
			}
		}
	}
	return nil
}

// First matching rule wins. Jump and aliases are ANDed; alias alternatives are ORed.
func (r RDPConfig) environmentFor(alias string, hops []hop) string {
	for _, env := range r.Environments {
		jumpMatches := env.Jump == ""
		for _, h := range hops {
			if ok, _ := path.Match(strings.ToLower(env.Jump), strings.ToLower(h.Host)); ok {
				jumpMatches = true
			}
		}
		aliasMatches := len(env.Aliases) == 0
		for _, pattern := range env.Aliases {
			if ok, _ := path.Match(strings.ToLower(pattern), strings.ToLower(alias)); ok {
				aliasMatches = true
			}
		}
		if jumpMatches && aliasMatches {
			return env.Name
		}
	}
	return ""
}
