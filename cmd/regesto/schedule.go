package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"regesto/internal/config"
)

// Two jobs, because they have different homes (DECISION §9.0): harvest runs on
// every machine against its own native memory, while the cycle runs only where
// write authority lives.
type job struct {
	label        string
	args         []string
	interval     int // seconds
	everyMachine bool
	why          string
}

func jobs(cfg *config.Config) []job {
	return []job{
		{
			label:        "com.regesto.harvest",
			args:         []string{"harvest"},
			interval:     900, // 15 minutes
			everyMachine: true,
			why:          "captures native memory; the interval is the loss window for the prune race",
		},
		{
			label:    "com.regesto.cycle",
			args:     []string{"cycle"},
			interval: 3600,
			why:      "normalise, reconcile, rebuild, commit — only where write authority lives",
		},
	}
}

func runSchedule(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("schedule", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	action := "status"
	if fs.NArg() > 0 {
		action = fs.Arg(0)
	}

	lintHost := strings.TrimSpace(cfg.Section("roles")["lint"])
	isLintHost := lintHost == "" || lintHost == cfg.Machine

	switch action {
	case "status":
		return scheduleStatus(cfg, isLintHost)
	case "print":
		for _, j := range jobs(cfg) {
			if !j.everyMachine && !isLintHost {
				continue
			}
			fmt.Printf("# %s — %s\n%s\n", j.label, j.why, plist(cfg, j))
		}
		return nil
	case "install":
		return scheduleInstall(cfg, isLintHost)
	case "uninstall":
		return scheduleUninstall(cfg)
	default:
		return fmt.Errorf("unknown action %q; use status, print, install or uninstall", action)
	}
}

func agentDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

func plistPath(label string) string { return filepath.Join(agentDir(), label+".plist") }

// plist renders a LaunchAgent. RunAtLoad is deliberately false for the cycle:
// loading it should not immediately spend money on a model.
func plist(cfg *config.Config, j job) string {
	bin := filepath.Join(cfg.KBRoot, "bin", "regesto")
	var argXML strings.Builder
	argXML.WriteString("\t\t<string>" + bin + "</string>\n")
	argXML.WriteString("\t\t<string>--config</string>\n")
	argXML.WriteString("\t\t<string>" + filepath.Join(cfg.KBRoot, "config.toml") + "</string>\n")
	for _, a := range j.args {
		argXML.WriteString("\t\t<string>" + a + "</string>\n")
	}
	logDir := filepath.Join(cfg.KBRoot, ".state", cfg.Machine, "logs")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>RunAtLoad</key>
	<false/>
	<key>StandardOutPath</key>
	<string>%s/%s.log</string>
	<key>StandardErrorPath</key>
	<string>%s/%s.err.log</string>
	<key>WorkingDirectory</key>
	<string>%s</string>
</dict>
</plist>
`, j.label, argXML.String(), j.interval, logDir, j.label, logDir, j.label, cfg.KBRoot)
}

func scheduleStatus(cfg *config.Config, isLintHost bool) error {
	fmt.Printf("machine %s (from %s)\n", cfg.Machine, cfg.MachineSource)
	if runtime.GOOS != "darwin" {
		fmt.Printf("scheduling here targets launchd; on %s use cron or a systemd timer with the same two commands\n", runtime.GOOS)
	}
	for _, j := range jobs(cfg) {
		if !j.everyMachine && !isLintHost {
			fmt.Printf("  %-20s not for this machine — [roles].lint names another host\n", j.label)
			continue
		}
		state := "not installed"
		if _, err := os.Stat(plistPath(j.label)); err == nil {
			state = "installed"
			if out, err := exec.Command("launchctl", "list", j.label).Output(); err == nil && len(out) > 0 {
				state = "installed and loaded"
			}
		}
		fmt.Printf("  %-20s every %4ds  %s\n", j.label, j.interval, state)
	}
	return nil
}

func scheduleInstall(cfg *config.Config, isLintHost bool) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("install targets launchd and this is %s; run `regesto schedule print` and adapt the two commands to cron or a systemd timer", runtime.GOOS)
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "bin", "regesto")); err != nil {
		return fmt.Errorf("bin/regesto is not built — run `go build -o bin/regesto ./cmd/regesto` first, or the jobs would fail every time they fire")
	}
	if err := os.MkdirAll(agentDir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfg.KBRoot, ".state", cfg.Machine, "logs"), 0o755); err != nil {
		return err
	}

	for _, j := range jobs(cfg) {
		if !j.everyMachine && !isLintHost {
			fmt.Printf("skip    %s — [roles].lint names another machine\n", j.label)
			continue
		}
		path := plistPath(j.label)
		if err := os.WriteFile(path, []byte(plist(cfg, j)), 0o644); err != nil {
			return err
		}
		// Unload first so a re-install picks up a changed interval or path.
		_ = exec.Command("launchctl", "unload", path).Run()
		if out, err := exec.Command("launchctl", "load", path).CombinedOutput(); err != nil {
			return fmt.Errorf("loading %s: %v: %s", j.label, err, strings.TrimSpace(string(out)))
		}
		fmt.Printf("loaded  %s — every %ds\n", j.label, j.interval)
	}
	fmt.Printf("logs in %s\n", filepath.Join(cfg.KBRoot, ".state", cfg.Machine, "logs"))
	return nil
}

func scheduleUninstall(cfg *config.Config) error {
	for _, j := range jobs(cfg) {
		path := plistPath(j.label)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		_ = exec.Command("launchctl", "unload", path).Run()
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", j.label)
	}
	return nil
}
