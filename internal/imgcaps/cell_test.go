package imgcaps

import (
	"context"
	"errors"
	"testing"
)

func TestCellFromWindow(t *testing.T) {
	tests := []struct {
		name                      string
		width, height, cols, rows int
		want                      Cell
		ok                        bool
	}{
		{"exact", 800, 480, 100, 30, Cell{8, 16}, true},
		{"padding rounds down", 1610, 975, 100, 30, Cell{16, 32}, true},
		{"no columns", 800, 480, 0, 30, Cell{}, false},
		{"no rows", 800, 480, 100, 0, Cell{}, false},
		{"no pixels", 0, 0, 100, 30, Cell{}, false},
		{"under a pixel", 50, 480, 100, 30, Cell{0, 16}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CellFromWindow(tt.width, tt.height, tt.cols, tt.rows)
			if got != tt.want || ok != tt.ok {
				t.Errorf("CellFromWindow(%d, %d, %d, %d) = %v, %v, want %v, %v",
					tt.width, tt.height, tt.cols, tt.rows, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestQueryTmuxClient(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		err     error
		want    TmuxClient
		wantErr bool
	}{
		{"kitty", "/dev/pts/3\tkitty(0.43.1)\t9\t18\ton\t1\n", nil, TmuxClient{TTY: "/dev/pts/3", Termtype: "kitty(0.43.1)", Passthrough: "on", Cell: Cell{9, 18}, Attached: 1}, false},
		{"a name with spaces", "/dev/pts/3\tghostty 1.2.0\t10\t20\tall\t1\n", nil, TmuxClient{TTY: "/dev/pts/3", Termtype: "ghostty 1.2.0", Passthrough: "all", Cell: Cell{10, 20}, Attached: 1}, false},
		{"no pixels", "/dev/pts/3\tkitty(0.43.1)\t0\t0\toff\t1\n", nil, TmuxClient{TTY: "/dev/pts/3", Termtype: "kitty(0.43.1)", Passthrough: "off", Attached: 1}, false},
		{"two clients", "/dev/pts/3\tkitty(0.43.1)\t9\t18\ton\t2\n", nil, TmuxClient{TTY: "/dev/pts/3", Termtype: "kitty(0.43.1)", Passthrough: "on", Cell: Cell{9, 18}, Attached: 2}, false},
		{"no client", "\t\t\t\t\t\n", nil, TmuxClient{}, false},
		{"too few fields", "/dev/pts/3\tkitty(0.43.1)\t9\t18\ton\n", nil, TmuxClient{}, true},
		{"tmux failed", "", errors.New("no server running"), TmuxClient{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var args []string
			got, err := QueryTmuxClient(t.Context(), func(_ context.Context, a ...string) (string, error) {
				args = a
				return tt.out, tt.err
			})
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("QueryTmuxClient = %+v, %v, want %+v, error %v", got, err, tt.want, tt.wantErr)
			}
			if len(args) != 3 || args[0] != "display-message" || args[1] != "-p" {
				t.Errorf("tmux ran with %q, want one display-message", args)
			}
		})
	}
}
