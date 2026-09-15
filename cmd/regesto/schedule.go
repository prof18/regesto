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
	title        string // what the job does, in plain words
	args         []string
	interval     int // seconds
	everyMachine bool
	why          string
}

func jobs(cfg *config.Config) []job {
	return []job{
		{
			label:        "com.regesto.harvest",
			title:        "Collect agent memories",
			args:         []string{"harvest"},
			interval:     900, // 15 minutes
			everyMachine: true,
			why:          "collects new agent memories into the inbox",
		},
		{
			label:    "com.regesto.cycle",
			title:    "Process the inbox and commit",
			args:     []string{"cycle"},
			interval: 3600,
			why:      "turns inbox notes into facts, tidies them, rebuilds the index and commits",
		},
	}
}

func runSchedule(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("schedule", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: regesto schedule [status|install|uninstall|print]\n\n"+
			"Run Regesto in the background: collect agent memories every 15 minutes and\n"+
			"process the inbox every hour (macOS launchd).\n\n"+
			"  status     Show the jobs and when processing last worked (default).\n"+
			"  install    Start the background jobs.\n"+
			"  uninstall  Stop and remove them.\n"+
			"  print      Print the job files, e.g. to adapt them for cron or systemd.")
	}
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
		return fmt.Errorf("unknown action %q; use status, install, uninstall or print", action)
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

	commands := normalize.CommandsFor("", cfg.Section("normalize")["commands"], cfg.IntegrationIDs())
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
	fmt.Printf("Background jobs on %s\n\n", cfg.Machine)
	if runtime.GOOS != "darwin" {
		fmt.Printf("  Background jobs use launchd, which %s doesn't have. Run `regesto schedule print`\n  and set up the same two commands with cron or a systemd timer.\n\n", runtime.GOOS)
	}
	for _, j := range jobs(cfg) {
		if !j.everyMachine && !isLintHost {
			fmt.Printf("  %-30s runs on another machine (see [roles] in config.toml)\n", j.title)
			continue
		}
		state := "not installed"
		if _, err := os.Stat(plistPath(j.label)); err == nil {
			state = "installed, not running"
			if out, err := exec.Command("launchctl", "list", j.label).Output(); err == nil && len(out) > 0 {
				state = "running"
			}
		}
		fmt.Printf("  %-30s %-16s %s\n", j.title, every(j.interval), state)
	}

	// Installed is not the same as working, and the difference is the whole
	// reason this section exists. The cycle reports its own failures (internal
	// /notify), but a job that never fires — unloaded, or holding a path to an
	// engine that has moved — reports nothing at all. A stale last clean pass is
	// the only evidence of that, so print it whether or not anything is wrong.
	fmt.Println()
	if isLintHost {
		if s, ok := notify.Load(cfg, "cycle"); ok {
			now := time.Now().UTC()
			switch {
			case s.Failing:
				fmt.Printf("  Last processing   FAILING for %s; run `regesto cycle` to see the error\n", ago(now.Sub(s.Since)))
			case s.LastOK.IsZero():
				fmt.Println("  Last processing   no successful run yet")
			default:
				fmt.Printf("  Last processing   worked %s ago\n", ago(now.Sub(s.LastOK)))
			}
		} else {
			fmt.Println("  Last processing   hasn't run on this machine yet")
		}
		if notify.Enabled(cfg) {
			fmt.Println("  Failure alerts    on")
		} else {
			fmt.Println("  Failure alerts    off (agents still see failures; set [notify] in config.toml for alerts)")
		}
	}
	// Which engine an install would schedule. Reported rather than fatal: a
	// status that refuses to print because no binary is installed yet is the
	// least useful moment to withhold the rest of it.
	if bin, err := engineBinary(cfg); err == nil {
		fmt.Printf("  Regesto command   %s\n", tildePath(bin))
	} else {
		fmt.Printf("  Regesto command   not found: %v\n", err)
	}
	return nil
}

// every renders a job interval as "every 15 min" or "every hour".
func every(seconds int) string {
	switch {
	case seconds == 3600:
		return "every hour"
	case seconds%3600 == 0:
		return fmt.Sprintf("every %d hours", seconds/3600)
	case seconds%60 == 0:
		return fmt.Sprintf("every %d min", seconds/60)
	default:
		return fmt.Sprintf("every %ds", seconds)
	}
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
		return fmt.Errorf("background jobs use macOS launchd, which %s doesn't have; run `regesto schedule print` and set up the same two commands with cron or a systemd timer", runtime.GOOS)
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
			fmt.Printf("Skipped   %s: it runs on another machine (see [roles] in config.toml)\n", j.title)
			continue
		}
		path := plistPath(j.label)
		if err := os.WriteFile(path, []byte(plist(cfg, j, bin)), 0o644); err != nil {
			return err
		}
		// Unload first so a re-install picks up a changed interval or path.
		_ = exec.Command("launchctl", "unload", path).Run()
		if out, err := exec.Command("launchctl", "load", path).CombinedOutput(); err != nil {
			return fmt.Errorf("starting %s (%s): %v: %s", j.title, j.label, err, strings.TrimSpace(string(out)))
		}
		fmt.Printf("Started   %s, %s\n", j.title, every(j.interval))
	}
	fmt.Printf("\nUsing   %s\n", tildePath(bin))
	fmt.Printf("Logs    %s\n", tildePath(filepath.Join(cfg.KBRoot, ".state", cfg.Machine, "logs")))
	return nil
}

func scheduleUninstall(cfg *config.Config) error {
	removed := 0
	for _, j := range jobs(cfg) {
		path := plistPath(j.label)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		_ = exec.Command("launchctl", "unload", path).Run()
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Printf("Removed   %s\n", j.title)
		removed++
	}
	if removed == 0 {
		fmt.Println("No background jobs were installed.")
	}
	return nil
}
