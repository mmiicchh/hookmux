// Package cli defines the hookmux command tree; each command is a type with a Run method.
package cli

import (
	"fmt"
	"io"
	"log"

	"github.com/alecthomas/kong"

	"github.com/mmiicchh/hookmux/internal/config"
	"github.com/mmiicchh/hookmux/internal/hist"
)

type CLI struct {
	Config  string           `help:"Config file." default:"${config}" env:"HOOKMUX_CONFIG" type:"path"`
	Version kong.VersionFlag `help:"Print version and exit."`

	Run   RunCmd   `cmd:"" help:"Run the first rule that matches."`
	Hist  HistCmd  `cmd:"" help:"List recent invocations, or show one in full."`
	View  ViewCmd  `cmd:"" help:"Serve the history as a local web page."`
	Check CheckCmd `cmd:"" help:"Compile the rules and verify their exec targets."`
}

// App is what main hands every command: config loaded once, the history store, the streams.
type App struct {
	Version    string
	Config     config.Config
	ConfigPath string
	ConfigErr  error // run fails on it; hist and view warn and use defaults
	Hist       hist.Store
	Log        *log.Logger // stderr, "hookmux: " prefix

	In       io.Reader
	Out, Err io.Writer
}

// warnConfig lets read-only commands proceed on a broken config: the history
// that recorded the breakage must stay reachable.
func (a *App) warnConfig() {
	if a.ConfigErr != nil {
		a.Log.Printf("config %s: %v (using defaults)", a.ConfigPath, a.ConfigErr)
	}
}

// ExitCode is returned by commands that must exit with a specific status.
type ExitCode int

func (e ExitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }
