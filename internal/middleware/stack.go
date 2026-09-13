package middleware

import "runtime"

// stack renders the goroutine stack for a recovered panic, skipping the frames
// belonging to the recovery machinery itself.
func stack() string {
	buf := make([]byte, 8<<10)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}
