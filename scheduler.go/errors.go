package scheduler

import "errors"

var (
	ErrInstallFailed     = errors.New("failed to install message")
	ErrorUninstallFailed = errors.New("failed to uninstall message")
)