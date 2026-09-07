package catalogui

import tea "github.com/charmbracelet/bubbletea"

// PickerResultMsg returns a selection or cancellation to the containing screen.
type PickerResultMsg struct {
	Value     string
	Cancelled bool
}

// NewEmbeddedPicker builds a picker for use inside another Bubble Tea model.
func NewEmbeddedPicker(entries []Entry) Model {
	model := NewPicker(entries)
	model.embedded = true
	return model
}

// EvidenceOpen lets a containing screen give paging to the evidence viewport
// rather than scrolling an outer recipe-body viewport.
func (model Model) EvidenceOpen() bool { return model.showDetail }

func (model Model) pickerCompletion() tea.Cmd {
	if !model.embedded {
		return tea.Quit
	}
	result := PickerResultMsg{Value: model.selected, Cancelled: model.cancelled}
	return func() tea.Msg { return result }
}
