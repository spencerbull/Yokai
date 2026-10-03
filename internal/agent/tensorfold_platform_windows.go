//go:build windows

package agent

import (
	"errors"
	"os"
	"os/exec"
)

var errTensorFoldUnsupportedPlatform = errors.New("TensorFold lifecycle requires a Unix host")

func tensorFoldConfigureCommand(_ *exec.Cmd) error {
	return errTensorFoldUnsupportedPlatform
}

func tensorFoldSignalProcessGroup(_ int, _ bool) error {
	return errTensorFoldUnsupportedPlatform
}

func tensorFoldExitWasIntentionalKill(_ error) bool {
	return false
}

func tensorFoldFileOwnedByCurrentUser(_ os.FileInfo) bool {
	return false
}
