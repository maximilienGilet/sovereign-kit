package catalogui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEmbeddedPickerCompletesWithTypedResult(t *testing.T) {
	for _, test := range []struct {
		name string
		key  tea.KeyMsg
		want PickerResultMsg
	}{
		{"choose", tea.KeyMsg{Type: tea.KeyEnter}, PickerResultMsg{Value: "studio"}},
		{"escape", tea.KeyMsg{Type: tea.KeyEsc}, PickerResultMsg{Cancelled: true}},
		{"q", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, PickerResultMsg{Cancelled: true}},
		{"interrupt", tea.KeyMsg{Type: tea.KeyCtrlC}, PickerResultMsg{Cancelled: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := NewEmbeddedPicker(pickerFixture())
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
			updated, cmd := updated.(Model).Update(test.key)
			if cmd == nil {
				t.Fatal("missing completion command")
			}
			msg := cmd()
			result, ok := msg.(PickerResultMsg)
			if !ok || result != test.want {
				t.Fatalf("completion = %#v, want %#v", msg, test.want)
			}
			if updated.(Model).Cancelled() != test.want.Cancelled {
				t.Fatal("cancel state differs from result")
			}
		})
	}
}

func TestEmbeddedEvidenceAndProgressRemainLocal(t *testing.T) {
	model := NewEmbeddedPicker(pickerFixture())
	cmd := model.Init()
	if cmd == nil {
		t.Fatal("embedded picker lost initial animation")
	}
	updated, _ := model.Update(cmd())
	model = updated.(Model)
	for _, close := range []tea.KeyMsg{{Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'i'}}} {
		updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		model = updated.(Model)
		if cmd != nil || !model.showDetail {
			t.Fatal("opening evidence completed picker")
		}
		updated, cmd = model.Update(close)
		model = updated.(Model)
		if cmd != nil || model.showDetail || model.Cancelled() {
			t.Fatal("closing evidence completed picker")
		}
	}
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := cmd().(PickerResultMsg); got.Value != "solo" || got.Cancelled {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestStandalonePickerStillQuits(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}} {
		_, cmd := NewPicker(pickerFixture()).Update(key)
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("standalone picker did not quit")
		}
	}
}
