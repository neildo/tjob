package tjob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	JailOp = ".tjob"
)

var (
	ErrInvalidArgs = errors.New("invalid args")
	ErrForceStop   = errors.New("force stop")
	startable      = false
)

type (
	Status struct {
		Pid       int
		Cmd       string
		StartedAt time.Time
		StoppedAt time.Time
		Ran       time.Duration
		Exit      int32 // exit code
		Error     error // go error
	}

	Job struct {
		// Unique Job Id
		Id string

		// Set underlying os/exec.Cmd.Path
		Path string

		// Set underlying os/exec.Cmd.Args.
		Args []string

		// $MAJ:$MIN device number for MNT namespace
		Mnt string

		// CPUPercent represents the quota of all cores.
		CPUPercent int

		// MemoryMB represents the quota of memory to in Megabytes.
		MemoryMB int

		// ReadBPS represents the max bytes read per second by proc
		ReadBPS int

		// WriteBPS represents the max bytes write per second by proc
		WriteBPS int

		// Read-write lock for job state
		rw sync.RWMutex

		// jail path to isolate process
		jailPath string

		// log file bind to os/exec.Cmd.Stdout and os/exec.Cmd.Stderr
		logs *os.File

		// cgroup file assigned to job
		cgroup *os.File

		status Status

		doneCh chan struct{} // closed when done running
	}

	JobReader struct {
		job     *Job
		logs    io.ReadCloser
		inotify *os.File
	}
)

func Init() error {
	// allow starting job
	startable = true

	if len(os.Args) < 3 && os.Args[1] == JailOp {
		return ErrInvalidArgs
	}
	// skip run command in jail
	if len(os.Args) < 3 || os.Args[1] != JailOp {
		return nil
	}
	fmt.Println(os.Args)
	// disallow starting job on jailed proc
	startable = false

	if err := mount(); err != nil {
		return err
	}
	args := os.Args[2:]
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return fmt.Errorf(cmd.ProcessState.String())
}

// Started return true if StartedAt is not zero
func (s Status) Started() bool {
	return !s.StartedAt.IsZero()
}

// Stopped return true if StoppedAt is not zero
func (s Status) Stopped() bool {
	return !s.StoppedAt.IsZero()
}

// Start starts the command
func (j *Job) Start(ctx context.Context) error {
	if !startable {
		panic("unsafe start")
	}
	// return any error from status if already stopped
	if j.Status().Stopped() {
		return j.status.Error
	}

	// already started nothing to do
	if j.Status().Started() {
		return nil
	}

	// jail the arbitrary process with required isolation
	cmd, err := jail(ctx, j)
	if err != nil {
		return fmt.Errorf("jail: %w", err)
	}
	// write stdout and stderr to log file
	logs, err := os.CreateTemp("", "*")
	if err != nil {
		return fmt.Errorf("log file: %w", err)
	}
	cmd.Stdout = logs
	cmd.Stderr = logs

	// start command
	j.rw.Lock()
	j.status.StartedAt = time.Now()
	if err := cmd.Start(); err != nil {
		j.rw.Unlock()
		logs.Close()
		j.status.Error = err
		j.status.StoppedAt = time.Now()
		return fmt.Errorf("start: %w", err)
	}
	j.logs = logs
	j.status.Pid = cmd.Process.Pid
	j.rw.Unlock()

	// wait on separate coroutine
	go j.wait(cmd)
	return nil
}

// wait waits for the process to stop
func (j *Job) wait(cmd *exec.Cmd) {
	defer close(j.doneCh)

	s, err := cmd.Process.Wait()
	now := time.Now()

	// Set final status
	j.rw.Lock()
	defer j.rw.Unlock()
	j.status.Ran = now.Sub(j.status.StartedAt)
	j.status.StoppedAt = now
	j.status.Exit = int32(s.ExitCode())

	if err != nil {
		err = errors.Join(j.status.Error, err)
	} else if !s.Success() {
		err = errors.Join(j.status.Error, &exec.ExitError{ProcessState: s})
	}
	j.status.Error = err

	// close log file
	j.logs.Close()

	// close cgroup file
	if j.cgroup != nil {
		j.cgroup.Close()
	}
}

// Wait waits for the process to stop
func (j *Job) Wait() error {
	<-j.doneCh
	return j.status.Error
}

// Stop signal SIGKILL on the process group and idempotent.
func (j *Job) Stop() error {
	j.rw.Lock()
	defer j.rw.Unlock()

	if j.status.Stopped() {
		return nil
	}
	j.status.Error = ErrForceStop

	return syscall.Kill(j.status.Pid, syscall.SIGKILL)
}

// Status returns the Status at any time and concurrency safe.
func (j *Job) Status() Status {
	j.rw.RLock()
	out := j.status

	// calculate ran duration
	if out.Ran == 0 {
		out.Ran = time.Now().Sub(out.StartedAt)
	}
	j.rw.RUnlock()
	return out
}

// Logs returns JobReader for polling logs until process stops
func (j *Job) Logs(ctx context.Context) (io.ReadCloser, error) {
	return NewJobReader(ctx, j)
}
