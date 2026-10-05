package script

import "context"

type ScreenSnapshot struct {
	Rows       int      `json:"rows"`
	Cols       int      `json:"cols"`
	CurrentRow int      `json:"currentRow"`
	Lines      []string `json:"lines"`
}

type scriptTextResult struct {
	text     string
	snapshot *ScreenSnapshot
}

func (r *Runner) SetScreenSnapshot(read func(context.Context, string) (ScreenSnapshot, error)) {
	r.mu.Lock()
	r.screenSnapshot = read
	r.mu.Unlock()
}
