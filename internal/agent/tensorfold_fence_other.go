//go:build !linux

package agent

import "errors"

// Launcher fencing needs /proc; elsewhere TensorFold cleanup fails closed.
func fenceTensorFoldProcesses(string) error {
	return errors.New("TensorFold launcher fencing requires Linux")
}
