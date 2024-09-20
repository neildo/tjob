package tjob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	jailOp = ".tjob"
)

// libState
const (
	notInited = iota
	startable
	jailed
)

// jobState
const (
	started = iota
	stopped
)

var (
	ErrAlreadyInited        = errors.New("already inited")
	ErrAlreadyStarted       = errors.New("already started")
	ErrNotStartable         = errors.New("not startable")
	ErrNotStarted           = errors.New("not started")
	ErrAlreadyJailed        = errors.New("already jailed")
	ErrInvalidArgs          = errors.New("invalid args")
	ErrForceStop            = errors.New("force stop")
	ErrReadAgain            = errors.New("read again")
	libState          int32 = notInited
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

		// log file bind to os/exec.Cmd.Stdout and os/exec.Cmd.Stderr
		logs *os.File

		// cgroup file assigned to job
		cgroup *os.File

		// closed when done running
		doneCh chan bool

		// started prevents the same process called twice
		state int32

		// jail path to isolate process
		jailPath string

		// Read-write lock for job status
		rw     sync.RWMutex
		status Status
	}

	Doner interface {
		Done() bool
	}
	JobReader struct {
		doner   Doner
		logs    *os.File
		inotify *os.File
	}
)

func Init() error {
	// MUST confirm Init() was called before starting any job
	if !atomic.CompareAndSwapInt32(&libState, notInited, startable) {
		return ErrAlreadyInited
	}

	if len(os.Args) < 3 && os.Args[1] == jailOp {
		return ErrInvalidArgs
	}
	// skip run command in jail
	if len(os.Args) < 3 || os.Args[1] != jailOp {
		return nil
	}
	// cannot start another job in the jailed state
	if !atomic.CompareAndSwapInt32(&libState, startable, jailed) {
		return ErrAlreadyJailed
	}
	if err := mount(); err != nil {
		return err
	}

	// run the arbitrary proc in jail
	args := os.Args[2:]
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	// return error to force early exit by caller
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
	// disallow starting another job unless safe
	if libState != startable {
		return ErrNotStartable
	}
	// prevent same proc starting this job twice
	if !atomic.CompareAndSwapInt32(&j.state, 0, started) {
		return ErrAlreadyStarted
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
		logs.Close()
		j.status.Error = err
		j.status.StoppedAt = time.Now()
		j.rw.Unlock()
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

	err := cmd.Wait()
	now := time.Now()

	// Set final status
	j.rw.Lock()
	defer j.rw.Unlock()
	j.status.Ran = now.Sub(j.status.StartedAt)
	j.status.StoppedAt = now
	if cmd.ProcessState != nil {
		j.status.Exit = int32(cmd.ProcessState.ExitCode())
	}

	if err != nil {
		err = errors.Join(j.status.Error, err)
	}
	j.status.Error = err

	// close log file
	j.logs.Close()

	// close cgroup file
	if j.cgroup != nil {
		j.cgroup.Close()
		// remove cgroup dir
		_ = unix.Rmdir(j.cgroup.Name())
	}
	atomic.CompareAndSwapInt32(&j.state, started, stopped)
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

// Done returns true if the Job completed
func (j *Job) Done() bool {
	return j.state == stopped
}

// Logs returns JobReader for polling logs until process stops
func (j *Job) Logs(ctx context.Context) (io.ReadCloser, error) {
	if j.logs == nil {
		return nil, ErrNotStartable
	}
	return NewJobReader(ctx, j.logs.Name(), j)
}
