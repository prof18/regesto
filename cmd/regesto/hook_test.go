package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestHookConfigFailuresUseHostValidFraming(t *testing.T) {
	for _, test := range []struct {
		protocol string
		want     string
	}{
		{protocol: "claude-session-start-v1", want: ""},
		{protocol: "hermes-pre-llm-v1", want: "{}"},
	} {
		t.Run(test.protocol, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := failOpenHook([]string{test.protocol}, &stdout, &stderr, errors.New("config unavailable")); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want || !strings.Contains(stderr.String(), "config unavailable") {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestUnknownHookDoesNotMasqueradeAsFailOpen(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := failOpenHook([]string{"unknown"}, &stdout, &stderr, errors.New("config unavailable"))
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("unknown hook: stdout=%q err=%v", stdout.String(), err)
	}
}
