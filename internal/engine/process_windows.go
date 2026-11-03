//go:build windows

package engine

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsProcessGroup struct {
	mu       sync.Mutex
	job      windows.Handle
	children []*exec.Cmd
	waits    map[*exec.Cmd]chan error
	wg       sync.WaitGroup
	waitErr  error
}

func newProcessGroup() (ProcessGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	return &windowsProcessGroup{job: job, waits: map[*exec.Cmd]chan error{}}, nil
}

// Start launches a process and immediately binds it to the job object.
// KILL_ON_JOB_CLOSE then guarantees that the whole tree dies with the parent.
//
// Documented limitation: a short window exists between cmd.Start and
// AssignProcessToJobObject during which the process is not yet in the job.
// Only the bundled utilities (curl, rclone, yt-dlp) are started here and none
// of them detach children, so the risk is acceptable. Closing the window
// completely would require creating the process suspended, which os/exec does
// not expose.
func (pg *windowsProcessGroup) Start(cmd *exec.Cmd) error {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	if pg.job == 0 {
		return errors.New("process group: already terminated")
	}
	// CREATE_NO_WINDOW keeps bundled tools (curl, rclone, yt-dlp) from
	// flashing console windows next to the UI.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process == nil {
		return nil
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	assignErr := windows.AssignProcessToJobObject(pg.job, h)
	windows.CloseHandle(h)
	if assignErr != nil {
		_ = cmd.Process.Kill()
		return assignErr
	}

	ch := make(chan error, 1)
	pg.waits[cmd] = ch
	pg.children = append(pg.children, cmd)

	// Reaper: releases the process handle and fills ProcessState.
	pg.wg.Add(1)
	go func(c *exec.Cmd) {
		defer pg.wg.Done()
		err := c.Wait()
		pg.mu.Lock()
		if err != nil && pg.waitErr == nil {
			pg.waitErr = err
		}
		pg.mu.Unlock()
		ch <- err
		close(ch)
	}(cmd)
	return nil
}

// WaitFor blocks until cmd has exited.
func (pg *windowsProcessGroup) WaitFor(cmd *exec.Cmd) error {
	pg.mu.Lock()
	ch, ok := pg.waits[cmd]
	pg.mu.Unlock()
	if !ok {
		return nil
	}
	return <-ch
}

// Terminate closes the job handle; KILL_ON_JOB_CLOSE kills every process.
func (pg *windowsProcessGroup) Terminate() error {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	if pg.job == 0 {
		return nil
	}
	err := windows.CloseHandle(pg.job)
	pg.job = 0
	return err
}

// Wait waits for all children. The mutex is not held while waiting: the reaper
// goroutines need it to finish.
func (pg *windowsProcessGroup) Wait() error {
	pg.wg.Wait()
	pg.mu.Lock()
	defer pg.mu.Unlock()
	return pg.waitErr
}
