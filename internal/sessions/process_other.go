//go:build !windows

package sessions

import "os/exec"

func configureCommand(cmd *exec.Cmd) {}
