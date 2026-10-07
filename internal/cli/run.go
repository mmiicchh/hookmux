package cli

import "github.com/mmiicchh/hookmux/internal/dispatch"

type RunCmd struct {
	Cmd []string `arg:"" optional:"" passthrough:"" help:"Hook name or command; rules see it as cmd."`
}

func (c *RunCmd) Run(app *App) error {
	rec := dispatch.Run(dispatch.Request{
		Version:    app.Version,
		Config:     app.Config,
		ConfigPath: app.ConfigPath,
		ConfigErr:  app.ConfigErr,
		Cmd:        c.Cmd,
		Hist:       app.Hist,
		Log:        app.Log,
		Stdin:      app.In,
		Stdout:     app.Out,
		Stderr:     app.Err,
	})
	if rec.Exit != 0 {
		return ExitCode(rec.Exit)
	}
	return nil
}
