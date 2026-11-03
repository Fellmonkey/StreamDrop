package engine

import "os/exec"

// ProcessGroup is an isolated group of child processes. Stopping the app or
// failing a task must never leave stray network processes behind.
//
// The contract is identical on both platforms: after Wait returns, no process
// of the group is running.
type ProcessGroup interface {
	// Start launches a process inside the group. Implementations reap the
	// process themselves (cmd.Wait in a goroutine), otherwise a finished
	// process stays a zombie and ProcessState never fills in.
	Start(cmd *exec.Cmd) error
	// WaitFor blocks until that specific process has been reaped.
	WaitFor(cmd *exec.Cmd) error
	// Terminate ends every process of the group.
	Terminate() error
	// Wait waits for every process of the group and returns the first error.
	Wait() error
}

// NewProcessGroup creates a platform process group: a Windows Job Object or a
// POSIX process group.
func NewProcessGroup() (ProcessGroup, error) {
	return newProcessGroup()
}
