package harness

import "fmt"

// safeCall runs fn and converts a panic into an error so a faulty component
// cannot crash the harness.
func safeCall(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn()
}

// resultTag turns an error into a metrics tag value.
func resultTag(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}
