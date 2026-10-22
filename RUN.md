# proto-actions

Throwaway prototype of a redesigned Actions modal, drawn with `pkg/bubbles/panes`
and the app's own theme (`config.Default().Palette`, `ui.NewTheme`, `ui.NewIcons`,
`ui.RunStyles`). Hardcoded data, no tests.

    go build -o /tmp/proto-actions ./cmd/proto-actions && /tmp/proto-actions

- `1` `2` `3`: variant (1 Runs | Jobs | Log, 2 Runs | Preview, 3 grouped Runs | Preview with status tabs)
- `tab`/`shift+tab` focus, `z` zoom (from 120 columns; narrower shows one pane with the strip), `[` `]` tab, `j`/`k` move or scroll, `g`/`G`, `q` quit
- `PROTO_LIGHT=1` or `PROTO_DARK=1` force the background; `PROTO_ICONS=unicode|ascii|nerd` the icon set
- Files: `main.go` (frame, header, strip, foot), `variants.go` (panes and rows), `data.go`
