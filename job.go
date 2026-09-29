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
	created = iota
	started
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
	libState          int32 = notInited //nolint:gochecknoglobals
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
		Id string //nolint:revive

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
		doneCh chan struct{}

		// started prevents the same process called twice
		state int32

		// jail path to isolate process
		jailPath string

		// Read-write lock for job status
		rw     sync.RWMutex
		status Status
	}

	// Doner reports whether the writer of a log file has finished. Done must
	// return true before the writer's final close of the file, because a
	// JobReader waiting at EOF relies on that close event to wake up and see it.
	Doner interface {
		Done() bool
	}
	JobReader struct {
		doner   Doner
		logs    *os.File
		inotify *os.File
		events  []byte

		// closed stops the context watcher when the reader is closed first
		closed    chan struct{}
		closeOnce sync.Once
		closeErr  error
	}
)

func Init() error {
	// MUST confirm Init() was called before starting any job
	if !atomic.CompareAndSwapInt32(&libState, notInited, startable) {
		return ErrAlreadyInited
	}

	if len(os.Args) == 2 && os.Args[1] == jailOp {
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
	err := cmd.Run()

	// Propagate the child's exit status as our own so the parent reports it.
	// A non-zero exit is not an error of the jail and must not be logged into
	// the job's output.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			os.Exit(128 + int(ws.Signal()))
		}
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	os.Exit(0)

	return nil
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
	if atomic.LoadInt32(&libState) != startable {
		return ErrNotStartable
	}
	// prevent same proc starting this job twice
	if !atomic.CompareAndSwapInt32(&j.state, created, started) {
		return ErrAlreadyStarted
	}

	// Hold the lock for the whole start so Status, Stop and Logs never observe
	// a half-started job. Everything below is local and bounded; no network.
	j.rw.Lock()
	defer j.rw.Unlock()

	j.status.StartedAt = time.Now()
	if err := j.start(ctx); err != nil {
		// A failed start is a finished job: record why, release Wait and
		// make Stop and Logs return ErrNotStarted.
		j.status.Error = err
		j.status.StoppedAt = time.Now()
		j.status.Ran = j.status.StoppedAt.Sub(j.status.StartedAt)
		atomic.StoreInt32(&j.state, stopped)
		close(j.doneCh)

		return err
	}
	return nil
}

// start runs the command and must be called with j.rw held.
func (j *Job) start(ctx context.Context) error {
	// jail the arbitrary process with required isolation
	cmd, cgroup, err := jail(ctx, j)
	if err != nil {
		return fmt.Errorf("jail: %w", err)
	}
	// write stdout and stderr to log file
	logs, err := os.CreateTemp("", "tjob-*.log")
	if err != nil {
		return errors.Join(fmt.Errorf("log file: %w", err), removeCgroup(cgroup))
	}
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		logs.Close()
		return errors.Join(fmt.Errorf("start: %w", err), removeCgroup(cgroup))
	}
	j.cgroup = cgroup
	j.logs = logs
	j.status.Pid = cmd.Process.Pid

	// wait on separate goroutine; it blocks on j.rw until Start returns
	go j.wait(cmd)

	return nil
}

// wait waits for the process to stop
func (j *Job) wait(cmd *exec.Cmd) {
	err := cmd.Wait()
	now := time.Now()

	// Set final status
	j.rw.Lock()
	j.status.Ran = now.Sub(j.status.StartedAt)
	j.status.StoppedAt = now
	if cmd.ProcessState != nil {
		j.status.Exit = int32(cmd.ProcessState.ExitCode())
	}
	if err != nil {
		err = errors.Join(j.status.Error, err)
	} else {
		err = j.status.Error
	}
	j.status.Error = errors.Join(err, removeCgroup(j.cgroup))
	j.rw.Unlock()

	// Order matters: mark stopped before the final close of the log file.
	// A reader at EOF that saw Done() == false is blocked on inotify and is
	// woken by this close; it must then see Done() == true. See Doner.
	atomic.StoreInt32(&j.state, stopped)
	j.logs.Close()
	close(j.doneCh)
}

// Wait waits for the process to stop
func (j *Job) Wait() error {
	<-j.doneCh

	j.rw.RLock()
	defer j.rw.RUnlock()
	return j.status.Error
}

// Stop sends SIGKILL to the jail, which is PID 1 of the job's PID namespace,
// so the kernel also kills every descendant. Stop is idempotent.
func (j *Job) Stop() error {
	j.rw.Lock()
	defer j.rw.Unlock()

	// Pid 0 would signal our own process group; never send it.
	if j.status.Pid == 0 {
		return ErrNotStarted
	}
	if j.status.Stopped() {
		return nil
	}
	if err := syscall.Kill(j.status.Pid, syscall.SIGKILL); err != nil {
		// ESRCH: exited but not yet reaped by wait; nothing left to stop.
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return fmt.Errorf("stop: %w", err)
	}
	j.status.Error = ErrForceStop
	return nil
}

// Status returns the Status at any time and concurrency safe.
func (j *Job) Status() Status {
	j.rw.RLock()
	out := j.status

	// calculate ran duration while running
	if out.Started() && !out.Stopped() {
		out.Ran = time.Since(out.StartedAt)
	}
	j.rw.RUnlock()
	return out
}

// Done returns true if the Job completed
func (j *Job) Done() bool {
	return atomic.LoadInt32(&j.state) == stopped
}

// Logs returns a JobReader that streams logs from the start until the job stops
func (j *Job) Logs(ctx context.Context) (io.ReadCloser, error) {
	j.rw.RLock()
	logs := j.logs
	j.rw.RUnlock()

	// no logs if never started or failed to start
	if logs == nil {
		return nil, ErrNotStarted
	}
	return NewJobReader(ctx, logs.Name(), j)
}
