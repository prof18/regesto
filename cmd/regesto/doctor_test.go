package main

import (
	"path/filepath"
	"testing"

	regestoinstall "github.com/prof18/regesto/internal/install"
)

func TestDoctorHookArtifactAssociation(t *testing.T) {
	first := doctorHookCapability{Protocol: "first-v1", Registrar: "manual", Settings: "/tmp/first.json"}
	second := doctorHookCapability{Protocol: "first-v1", Registrar: "manual", Settings: "/tmp/second.json"}
	item := regestoinstall.Item{ID: "hook-manual:first-v1:/tmp/first.json", DeclaredTargets: []string{"/tmp/first.json"}}
	if !hookArtifactMatches(first, item) {
		t.Fatal("matching hook artifact was not associated with its protocol/target")
	}
	if hookArtifactMatches(second, item) {
		t.Fatal("hook artifact leaked into a different hook capability")
	}

	hermes := doctorHookCapability{Protocol: "hermes-pre-llm-v1", Registrar: "hermes-config-yaml-v1", Settings: "/tmp/hermes/config.yaml"}
	allowlist := regestoinstall.Item{ID: "hook-hermes-allowlist", DeclaredTargets: []string{filepath.Join("/tmp/hermes", "shell-hooks-allowlist.json")}}
	if !hookArtifactMatches(hermes, allowlist) {
		t.Fatal("Hermes allowlist was not associated with the Hermes hook")
	}
}
