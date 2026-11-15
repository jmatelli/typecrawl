package app

import "testing"

// The typing view is redrawn on every keystroke, so the strip must not
// make that noticeably slower.
func BenchmarkTypingView(b *testing.B) {
	for _, on := range []bool{false, true} {
		name := "strip off"
		if on {
			name = "strip on"
		}
		b.Run(name, func(b *testing.B) {
			m := newBattleModel("the", "quick", "brown", "fox", "jumps", "over", "the", "lazy", "dog")
			m.battle = on
			m = typeString(m, "qu")
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}
