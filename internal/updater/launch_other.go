//go:build !windows

package updater

import "os/exec"

func launch(path string, args string) error {
	return exec.Command(path, args).Start()
}
