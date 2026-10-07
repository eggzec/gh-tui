package filterform

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows a form the way a section would: it focuses it, keeps what the
// user applies, and quits when the form is done.
type host struct {
	form      Model
	applied   *AppliedMsg
	cancelled bool
}

func (h host) Init() tea.Cmd { return h.form.Init() }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.form.SetSize(msg.Width, msg.Height)
		return h, nil
	case AppliedMsg:
		if msg.ID == h.form.ID() {
			h.applied = &msg
			return h, tea.Quit
		}
	case CancelMsg:
		if msg.ID == h.form.ID() {
			h.cancelled = true
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.form, cmd = h.form.Update(msg)
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.form.View()) }

// staticSpec is prSpec with fixed labels, so no key waits on a load.
func staticSpec() Spec {
	s := prSpec(nil)
	s.Fields[rowLabels].Options = labels
	return s
}

func TestProgram(t *testing.T) {
	tests := []struct {
		name          string
		keys          []tea.KeyPressMsg
		typed         string
		wantQuery     string
		wantCancelled bool
	}{
		{
			name: "choose and apply",
			// Closed, then docs from the labels' picker, then drafts off.
			keys: []tea.KeyPressMsg{
				right, down, down, down, space, down, down, space, enter, down, space, enter,
			},
			wantQuery: "is:closed author:@me review-requested:@me label:bug,enhancement,docs -is:draft base:main sort:updated-desc",
		},
		{
			name:      "type in the query line",
			keys:      []tea.KeyPressMsg{keyBigG, keyA, ctrlU},
			typed:     `is:merged label:"good first issue" crash`,
			wantQuery: `is:merged label:"good first issue" sort:updated-desc crash`,
		},
		{name: "cancel", keys: []tea.KeyPressMsg{right, esc}, wantCancelled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(staticSpec())
			m.Focus()
			tm := teatest.NewTestModel(t, host{form: m}, teatest.WithInitialTermSize(60, 20))
			for _, k := range tt.keys {
				tm.Send(k)
			}
			if tt.typed != "" {
				tm.Type(tt.typed)
				tm.Send(enter)
			}
			final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
			if !ok {
				t.Fatal("final model is not a host")
			}
			if final.cancelled != tt.wantCancelled {
				t.Errorf("cancelled = %v, want %v", final.cancelled, tt.wantCancelled)
			}
			if tt.wantCancelled {
				return
			}
			if final.applied == nil {
				t.Fatal("nothing was applied")
			}
			if final.applied.Query != tt.wantQuery {
				t.Errorf("applied %q, want %q", final.applied.Query, tt.wantQuery)
			}
			if final.form.Query() != tt.wantQuery {
				t.Errorf("the form holds %q, want %q", final.form.Query(), tt.wantQuery)
			}
		})
	}
}
