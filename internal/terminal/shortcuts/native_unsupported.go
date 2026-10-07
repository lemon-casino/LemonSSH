//go:build (!windows && !linux && !darwin) || (!cgo && !windows)

package shortcuts

import "fmt"

func RegisterNative(string, func(), func(func())) (func() error, error) {
	return nil, fmt.Errorf("native global shortcuts unavailable in this build")
}
