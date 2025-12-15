package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghostlawless/xdl/internal/log"
)

type RunContext struct {
	Users             []string
	Mode              RunMode
	RunID             string
	RunSeed           []byte
	LogPath           string
	CookiePath        string
	CookiePersistPath string
	OutRoot           string
	NoDownload        bool
	DryRun            bool
}

type RunMode int

func p9() string {
	p0, e0 := os.Executable()
	if e0 != nil || strings.TrimSpace(p0) == "" {
		return "."
	}
	if p1, e1 := filepath.EvalSymlinks(p0); e1 == nil && strings.TrimSpace(p1) != "" {
		p0 = p1
	}
	d0 := filepath.Dir(p0)
	if strings.TrimSpace(d0) == "" {
		return "."
	}
	return d0
}

func Run() {
	if err := RunWithArgsAndID(os.Args[1:], "", nil); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
	}
}

func RunWithArgs(args []string) error {
	return RunWithArgsAndID(args, "", nil)
}

func RunWithArgsAndID(args []string, runID string, runSeed []byte) error {
	r0, e0 := parseArgs(args, runID, runSeed)
	if e0 != nil {
		return e0
	}
	return runWithContext(r0)
}

func parseArgs(a0 []string, p0 string, p1 []byte) (RunContext, error) {
	a1 := make([]string, 0, len(a0))
	for _, a2 := range a0 {
		switch a2 {
		case "/d":
			a1 = append(a1, "-d")
		case "/q":
			a1 = append(a1, "-q")
		default:
			a1 = append(a1, a2)
		}
	}

	var (
		v0 bool
		v1 bool
	)

	z0 := flag.NewFlagSet("xdl", flag.ContinueOnError)
	z0.SetOutput(io.Discard)
	z0.BoolVar(&v0, "q", false, "Quiet mode")
	z0.BoolVar(&v1, "d", false, "Debug mode")

	if e0 := z0.Parse(a1); e0 != nil {
		return RunContext{}, fmt.Errorf(
			"Invalid arguments: %v\n\nUsage:\n  xdl [-q|-d] <username> [more_usernames...]\n\nExamples:\n  xdl google\n  xdl google nasa\n  xdl -d google",
			e0,
		)
	}

	u0 := make([]string, 0, len(z0.Args()))
	for _, u1 := range z0.Args() {
		u2 := strings.TrimSpace(u1)
		if u2 == "" {
			continue
		}
		u0 = append(u0, u2)
	}

	if len(u0) == 0 {
		return RunContext{}, fmt.Errorf(
			"Missing username.\n\nUsage:\n  xdl [-q|-d] <username> [more_usernames...]\n\nExamples:\n  xdl google\n  xdl google nasa\n  xdl -d google",
		)
	}

	r0 := RunContext{
		Users:      u0,
		Mode:       ModeVerbose,
		RunID:      p0,
		RunSeed:    p1,
		OutRoot:    "xDownloads",
		NoDownload: false,
		DryRun:     false,
	}

	if v1 {
		r0.Mode = ModeDebug
	} else if v0 {
		r0.Mode = ModeQuiet
	}

	if r0.RunID == "" {
		r0.RunID = generateRunID()
	}

	if r0.Mode == ModeDebug {
		m0 := "multi"
		if len(r0.Users) == 1 && strings.TrimSpace(r0.Users[0]) != "" {
			m0 = r0.Users[0]
		}

		r0.LogPath = filepath.Join(p9(), "debug", "run_"+m0+"_"+r0.RunID)
		if e1 := os.MkdirAll(r0.LogPath, 0o755); e1 != nil {
			return RunContext{}, fmt.Errorf("Could not create debug folder: %w", e1)
		}
		log.Init(filepath.Join(r0.LogPath, "main.log"))
		log.LogInfo("main", "Debug mode enabled; logs stored in "+r0.LogPath)
	} else {
		log.Disable()
	}

	return r0, nil
}
