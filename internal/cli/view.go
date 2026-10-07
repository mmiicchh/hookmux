package cli

import "github.com/mmiicchh/hookmux/internal/view"

type ViewCmd struct {
	NoBrowser bool `help:"Print the URL instead of opening a browser."`
}

func (c *ViewCmd) Run(app *App) error {
	app.warnConfig()
	return view.Serve(view.Options{
		Hist:        app.Hist,
		Port:        app.Config.ViewPort,
		OpenBrowser: !c.NoBrowser,
		Log:         app.Out,
	})
}
