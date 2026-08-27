package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/normalize"
	"github.com/prof18/regesto/internal/notify"
)

// Two jobs, because they have different homes (DESIGN §9.0): harvest runs on
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
		bin, err := engineBinary(cfg)
		if err != nil {
			return err
		}
		for _, j := range jobs(cfg) {
			if !j.everyMachine && !isLintHost {
				continue
			}
			fmt.Printf("# %s — %s\n%s\n", j.label, j.why, plist(cfg, j, bin))
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

// engineBinary is the path launchd will invoke. The resolution lives in config
// because "which engine serves this instance" is an instance fact, not a
// scheduling one.
func engineBinary(cfg *config.Config) (string, error) { return config.ResolveEngine(cfg) }

// schedulePath is deliberately narrower than the installer's entire PATH.
// Agent sessions and version managers often prepend temporary directories; a
// LaunchAgent outlives those directories and must not preserve them. Keep the
// paths of commands that are actually configured, then add stable user,
// Homebrew and system locations. [schedule].extra_path covers uncommon layouts.
func schedulePath(cfg *config.Config, bin string) string {
	var dirs []string
	dirs = append(dirs, filepath.SplitList(cfg.Section("schedule")["extra_path"])...)
	dirs = append(dirs, filepath.Dir(bin))

	commands := normalize.Commands("", cfg.Section("normalize")["commands"])
	if command := strings.TrimSpace(cfg.Section("notify")["command"]); command != "" {
		commands = append(commands, command)
	}
	for _, command := range commands {
		fields := strings.Fields(command)
		if len(fields) == 0 {
			continue
		}
		if resolved, err := exec.LookPath(fields[0]); err == nil {
			dirs = append(dirs, filepath.Dir(resolved))
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"))
	}
	dirs = append(dirs,
		"/opt/homebrew/bin", "/opt/homebrew/sbin",
		"/usr/local/bin", "/usr/local/sbin",
		"/usr/bin", "/bin", "/usr/sbin", "/sbin",
	)

	seen := map[string]bool{}
	unique := dirs[:0]
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		unique = append(unique, dir)
	}
	return strings.Join(unique, string(os.PathListSeparator))
}

func xmlText(s string) string { return html.EscapeString(s) }

// plist renders a LaunchAgent. RunAtLoad is deliberately false for the cycle:
// loading it should not immediately spend money on a model.
func plist(cfg *config.Config, j job, bin string) string {
	var argXML strings.Builder
	argXML.WriteString("\t\t<string>" + xmlText(bin) + "</string>\n")
	argXML.WriteString("\t\t<string>--config</string>\n")
	argXML.WriteString("\t\t<string>" + xmlText(filepath.Join(cfg.KBRoot, "config.toml")) + "</string>\n")
	for _, a := range j.args {
		argXML.WriteString("\t\t<string>" + xmlText(a) + "</string>\n")
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
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>%s</string>
	</dict>
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
`, xmlText(j.label), argXML.String(), xmlText(schedulePath(cfg, bin)), j.interval,
		xmlText(logDir), xmlText(j.label), xmlText(logDir), xmlText(j.label), xmlText(cfg.KBRoot))
}

func scheduleStatus(cfg *config.Config, isLintHost bool) error {
	fmt.Printf("machine %s (from %s)\n", cfg.Machine, cfg.MachineSource)
	// Which engine an install would schedule. Reported rather than fatal: a
	// status that refuses to print because no binary is installed yet is the
	// least useful moment to withhold the rest of it.
	if bin, err := engineBinary(cfg); err == nil {
		fmt.Printf("engine  %s\n", bin)
	} else {
		fmt.Printf("engine  none usable — %v\n", err)
	}
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

	// Installed is not the same as working, and the difference is the whole
	// reason this section exists. The cycle reports its own failures (internal
	// /notify), but a job that never fires — unloaded, or holding a path to an
	// engine that has moved — reports nothing at all. A stale last clean pass is
	// the only evidence of that, so print it whether or not anything is wrong.
	if isLintHost {
		if s, ok := notify.Load(cfg, "cycle"); ok {
			now := time.Now().UTC()
			switch {
			case s.Failing:
				fmt.Printf("  %-20s FAILING for %s\n", "cycle health", ago(now.Sub(s.Since)))
			case s.LastOK.IsZero():
				fmt.Printf("  %-20s no clean pass recorded yet\n", "cycle health")
			default:
				fmt.Printf("  %-20s last clean pass %s ago\n", "cycle health", ago(now.Sub(s.LastOK)))
			}
		} else {
			fmt.Printf("  %-20s no pass recorded yet on this machine\n", "cycle health")
		}
		if !notify.Enabled(cfg) {
			fmt.Printf("  %-20s off — failures will be silent; see [notify] in config.toml\n", "notifications")
		}
	}
	return nil
}

// ago renders a duration the way someone reading a status line wants it: one
// unit, largest that fits.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func scheduleInstall(cfg *config.Config, isLintHost bool) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("install targets launchd and this is %s; run `regesto schedule print` and adapt the two commands to cron or a systemd timer", runtime.GOOS)
	}
	bin, err := engineBinary(cfg)
	if err != nil {
		return err
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
		if err := os.WriteFile(path, []byte(plist(cfg, j, bin)), 0o644); err != nil {
			return err
		}
		// Unload first so a re-install picks up a changed interval or path.
		_ = exec.Command("launchctl", "unload", path).Run()
		if out, err := exec.Command("launchctl", "load", path).CombinedOutput(); err != nil {
			return fmt.Errorf("loading %s: %v: %s", j.label, err, strings.TrimSpace(string(out)))
		}
		fmt.Printf("loaded  %s — every %ds\n", j.label, j.interval)
	}
	fmt.Printf("engine  %s\n", bin)
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
