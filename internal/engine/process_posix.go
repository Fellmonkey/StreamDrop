//go:build !windows

package engine

import (
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type posixProcessGroup struct {
	mu       sync.Mutex
	pgid     int
	children []*exec.Cmd
	waits    map[*exec.Cmd]chan error
	wg       sync.WaitGroup
	waitErr  error
	running  int
}

func newProcessGroup() (ProcessGroup, error) {
	return &posixProcessGroup{waits: map[*exec.Cmd]chan error{}}, nil
}

// Start puts the process into a shared process group. The first process becomes
// the leader (its pid is the pgid); later processes join it.
func (pg *posixProcessGroup) Start(cmd *exec.Cmd) error {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	if pg.pgid != 0 {
		cmd.SysProcAttr.Pgid = pg.pgid
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if pg.pgid == 0 {
		pg.pgid = cmd.Process.Pid
	}
	ch := make(chan error, 1)
	pg.waits[cmd] = ch
	pg.children = append(pg.children, cmd)
	pg.running++

	// Reaper: releases the process and fills ProcessState. The running counter
	// avoids reading ProcessState concurrently with Wait (a data race).
	pg.wg.Add(1)
	go func(c *exec.Cmd) {
		defer pg.wg.Done()
		err := c.Wait()
		pg.mu.Lock()
		pg.running--
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
func (pg *posixProcessGroup) WaitFor(cmd *exec.Cmd) error {
	pg.mu.Lock()
	ch, ok := pg.waits[cmd]
	pg.mu.Unlock()
	if !ok {
		return nil
	}
	return <-ch
}

// Terminate sends SIGTERM to the group, then SIGKILL if it is still alive.
func (pg *posixProcessGroup) Terminate() error {
	pg.mu.Lock()
	pgid := pg.pgid
	pg.mu.Unlock()
	if pgid == 0 {
		return nil
	}

	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !pg.hasRunning() {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}

func (pg *posixProcessGroup) hasRunning() bool {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	return pg.running > 0
}

func (pg *posixProcessGroup) Wait() error {
	pg.wg.Wait()
	pg.mu.Lock()
	defer pg.mu.Unlock()
	return pg.waitErr
}
