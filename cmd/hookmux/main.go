// hookmux routes and records AI-agent hook invocations.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/mmiicchh/hookmux/internal/cli"
	"github.com/mmiicchh/hookmux/internal/config"
	"github.com/mmiicchh/hookmux/internal/hist"
)

// version may be set at build time with -ldflags "-X main.version=v1.2.3".
// Unset, Go's build info supplies the module version (`go install …@v1.2.3`),
// the tag of a tagged checkout, or a pseudo-version for other local builds.
var version string

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func main() {
	var c cli.CLI
	v := buildVersion()
	parser := kong.Must(&c,
		kong.Name("hookmux"),
		kong.Description("Route a command invocation to another command by rules and record it. "+
			"Built for AI-agent hooks, works for any command that takes stdin."),
		kong.Vars{"version": v, "config": config.DefaultPath(), "histLimit": strconv.Itoa(hist.DefaultLimit)},
		kong.UsageOnError(),
	)
	logger := log.New(os.Stderr, "hookmux: ", 0)
	args := os.Args[1:]
	ctx, err := parser.Parse(args)
	if err != nil {
		os.Exit(parseError(logger, args, err))
	}
	cfg, cfgErr := config.Load(c.Config)
	err = ctx.Run(&cli.App{
		Version: v,
		Config:  cfg, ConfigPath: c.Config, ConfigErr: cfgErr,
		Hist: hist.Store{Dir: cfg.HistDir, MaxKB: cfg.HistMaxKB, KeepDays: cfg.HistKeepDays},
		Log:  logger,
		In:   os.Stdin, Out: os.Stdout, Err: os.Stderr,
	})
	var code cli.ExitCode
	switch {
	case err == nil:
	case errors.As(err, &code):
		os.Exit(int(code))
	default:
		logger.Println(err)
		os.Exit(1)
	}
}

func parseError(logger *log.Logger, args []string, err error) int {
	if len(args) == 0 {
		logger.Println("expected a command (run, hist, view, check); see hookmux --help")
		return 2
	}
	logger.Println(err)
	if rest := positionals(args); strings.HasPrefix(err.Error(), "unexpected argument") && len(rest) > 0 {
		fmt.Fprintf(os.Stderr, "to wrap a command, put \"run\" before it: hookmux run %s\n", strings.Join(rest, " "))
	}
	return 2
}

// positionals drops the leading global flags (and --config's value) from args.
func positionals(args []string) []string {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		if args[0] == "--config" && len(args) > 1 {
			args = args[1:]
		}
		args = args[1:]
	}
	return args
}
