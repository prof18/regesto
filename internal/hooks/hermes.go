package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/config"
)

const (
	hermesMarkerDir = ".state/hooks/hermes-sessions"
	hermesMarkerMax = 256
)

type hermesPayload struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	Extra         struct {
		IsFirstTurn *bool `json:"is_first_turn"`
	} `json:"extra"`
}

func runHermes(cfg *config.Config, body []byte) (string, error) {
	var payload hermesPayload
	if err := decodePayload(body, &payload); err != nil {
		return "", err
	}
	if payload.HookEventName != "" && payload.HookEventName != "pre_llm_call" {
		return "", nil
	}
	dir := usableDirectory(payload.CWD)
	if dir == "" {
		return "", fmt.Errorf("hook payload contains no usable cwd")
	}
	if payload.Extra.IsFirstTurn != nil && !*payload.Extra.IsFirstTurn {
		return "", nil
	}
	if payload.SessionID == "" {
		if payload.Extra.IsFirstTurn == nil {
			return "", nil
		}
	}

	// Generate context before claiming the session. A transient knowledge-base
	// failure must not consume the only first-turn injection opportunity.
	context, err := buildContext(cfg, dir)
	if err != nil {
		return "", err
	}
	if payload.SessionID == "" {
		return context, nil
	}
	claimed, err := claimHermesSession(cfg.KBRoot, payload.SessionID)
	if err != nil {
		return "", err
	}
	if !claimed {
		return "", nil
	}
	return context, nil
}

func claimHermesSession(kbRoot, sessionID string) (bool, error) {
	root, err := os.OpenRoot(kbRoot)
	if err != nil {
		return false, err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.FromSlash(hermesMarkerDir), 0o700); err != nil {
		return false, err
	}
	hash := sha256.Sum256([]byte(sessionID))
	name := hex.EncodeToString(hash[:])
	marker := filepath.Join(filepath.FromSlash(hermesMarkerDir), name)
	file, err := root.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		root.Remove(marker)
		return false, err
	}
	if err := file.Close(); err != nil {
		root.Remove(marker)
		return false, err
	}
	if err := pruneHermesMarkers(root, marker); err != nil {
		root.Remove(marker)
		return false, err
	}
	return true, nil
}

type hermesMarker struct {
	name string
	when time.Time
}

func pruneHermesMarkers(root *os.Root, newest string) error {
	dir, err := root.Open(filepath.FromSlash(hermesMarkerDir))
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	markers := make([]hermesMarker, 0, len(entries))
	for _, entry := range entries {
		if !validHermesMarkerName(entry.Name()) || entry.Type()&os.ModeType != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		markers = append(markers, hermesMarker{name: entry.Name(), when: info.ModTime()})
	}
	if len(markers) <= hermesMarkerMax {
		return nil
	}
	sort.Slice(markers, func(i, j int) bool {
		if markers[i].when.Equal(markers[j].when) {
			return markers[i].name < markers[j].name
		}
		return markers[i].when.Before(markers[j].when)
	})
	remove := len(markers) - hermesMarkerMax
	for _, marker := range markers {
		path := filepath.Join(filepath.FromSlash(hermesMarkerDir), marker.name)
		if path == newest {
			continue
		}
		if err := root.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		remove--
		if remove == 0 {
			break
		}
	}
	return nil
}

func validHermesMarkerName(name string) bool {
	if len(name) != sha256.Size*2 || strings.ContainsAny(name, "/\\") {
		return false
	}
	decoded, err := hex.DecodeString(name)
	return err == nil && len(decoded) == sha256.Size
}
