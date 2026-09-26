package terminal

const (
	statusRows       = 5
	preferredColumns = 80
	meterMinColumns  = 54
)

type statusLayout struct {
	rows, columns int
}

// A single policy for reservation, drawing and visibility. Never keep a wider
// canvas after a shrink, and never reserve rows for an invisible component.
func layoutFor(columns, rows int, enabled bool) statusLayout {
	if !enabled || columns < meterMinColumns || rows < statusRows+2 {
		return statusLayout{}
	}
	return statusLayout{rows: statusRows, columns: min(preferredColumns, columns)}
}
