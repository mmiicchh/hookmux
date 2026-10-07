package cli

import "fmt"

type HistCmd struct {
	Row   int  `arg:"" optional:"" help:"Row number from the list; shows that invocation in full."`
	Limit int  `default:"${histLimit}" help:"Rows to list."`
	Clear bool `help:"Delete the history instead of listing it."`
}

func (c *HistCmd) Run(app *App) error {
	app.warnConfig()
	if c.Clear {
		return app.Hist.Clear()
	}
	recs, err := app.Hist.List(max(c.Limit, c.Row))
	if err != nil {
		return err
	}
	if c.Row == 0 {
		return templates().ExecuteTemplate(app.Out, "hist_list", recs)
	}
	if c.Row < 1 || c.Row > len(recs) {
		return fmt.Errorf("row %d out of range 1..%d", c.Row, len(recs))
	}
	return templates().ExecuteTemplate(app.Out, "hist_detail", recs[c.Row-1])
}
