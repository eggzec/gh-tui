//go:build race

package markdown

// slowdown is how much slower the race detector makes code, which time
// limits allow for.
const slowdown = 10
