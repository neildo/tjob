package tjob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const (
	cgroupRoot     = "/sys/fs/cgroup"
	cpuPeriod      = 100000
	cgroupFileMode = 0o500
)

// NewJob creates Job for the given command path and args until Start()
func NewJob(path string, args ...string) *Job {
	status := Status{
		Cmd: strings.Join(append([]string{path}, args...), " "),
	}
	return &Job{
		Id:       uuid.New().String(),
		jailPath: "/proc/self/exe",
		Path:     path,
		Args:     args,
		status:   status,
		doneCh:   make(chan struct{}),
	}
}

func mount() error {
	// MUST override the parent /proc before running command. linux unmount upon exit
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("mount: %w", err)
	}
	return nil
}

// jail creates the cgroup and namespaces required by the job to isolate
// exec.Cmd. The caller owns the returned cgroup and must removeCgroup it.
func jail(ctx context.Context, job *Job) (*exec.Cmd, *os.File, error) {
	// The job's process joins this cgroup directly, so it must be a leaf.
	cgroupJob := fmt.Sprintf("%s/%s", cgroupRoot, job.Id)
	if err := os.Mkdir(cgroupJob, cgroupFileMode); err != nil {
		return nil, nil, fmt.Errorf("mkdir %s: %w", cgroupJob, err)
	}
	if err := limit(job, cgroupJob); err != nil {
		return nil, nil, errors.Join(err, unix.Rmdir(cgroupJob))
	}

	// open cgroup file to jail clone
	cgroup, err := os.OpenFile(cgroupJob, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("%s: %w", cgroupJob, err), unix.Rmdir(cgroupJob))
	}

	args := append([]string{jailOp, job.Path}, job.Args...)
	cmd := exec.CommandContext(ctx, job.jailPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
		CgroupFD:     int(cgroup.Fd()),
		UseCgroupFD:  true,
	}
	return cmd, cgroup, nil
}

// limit writes the job's resource limits into its cgroup. A zero limit
// means unlimited, so the zero value of Job never OOM-kills or fails a job.
func limit(job *Job, cgroupJob string) error {
	// enable cpu, io, and memory controllers
	path := cgroupRoot + "/cgroup.subtree_control"
	if err := os.WriteFile(path, []byte("+cpu +io +memory"), cgroupFileMode); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// limit cpu
	if job.CPUPercent > 0 {
		n := float32(job.CPUPercent) / 100 * cpuPeriod
		content := fmt.Sprintf("%d %d", int(n), cpuPeriod)
		path = cgroupJob + "/cpu.max"
		if err := os.WriteFile(path, []byte(content), cgroupFileMode); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	// limit memory
	if job.MemoryMB > 0 {
		path = cgroupJob + "/memory.max"
		if err := os.WriteFile(path, []byte(fmt.Sprintf("%dM", job.MemoryMB)), cgroupFileMode); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	// limit rbps and wbps on the device, if one is given
	if job.Mnt != "" {
		content := fmt.Sprintf("%s rbps=%s wbps=%s riops=max wiops=max", job.Mnt, bps(job.ReadBPS), bps(job.WriteBPS))
		path = cgroupJob + "/io.max"
		if err := os.WriteFile(path, []byte(content), cgroupFileMode); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

// bps formats a bytes per second limit for io.max, where 0 means unlimited.
func bps(n int) string {
	if n <= 0 {
		return "max"
	}
	return strconv.Itoa(n)
}

// removeCgroup closes and removes the job's cgroup. The cgroup must be empty,
// which holds once the jail (PID 1 of the job's namespace) has been reaped.
func removeCgroup(cgroup *os.File) error {
	if cgroup == nil {
		return nil
	}
	return errors.Join(cgroup.Close(), unix.Rmdir(cgroup.Name()))
}

// NewJobReader returns the io.ReadCloser
func NewJobReader(ctx context.Context, filename string, doner Doner) (io.ReadCloser, error) {
	log, err := os.OpenFile(filename, os.O_RDONLY, 0o660)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	// Reader specific notify. IN_NONBLOCK lets os.File use the runtime poller,
	// so Close reliably unblocks a Read that is waiting for events.
	desc, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("inotify_init1: %w", err), log.Close())
	}
	// watch for writes and close event
	_, err = syscall.InotifyAddWatch(desc, log.Name(), syscall.IN_MODIFY|syscall.IN_CLOSE)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("inotify_add_watch: %w", err), syscall.Close(desc), log.Close())
	}
	r := &JobReader{
		doner:   doner,
		logs:    log,
		inotify: os.NewFile(uintptr(desc), log.Name()),
		events:  make([]byte, syscall.SizeofInotifyEvent+syscall.NAME_MAX+1),
		closed:  make(chan struct{}),
	}
	// close to unblock reads if context is done; exit if closed first
	go func() {
		select {
		case <-ctx.Done():
			_ = r.Close()
		case <-r.closed:
		}
	}()
	return r, nil
}

// Read reads n bytes into buffer and return EOF only when Job stops
func (r *JobReader) Read(buffer []byte) (n int, err error) { //nolint:nonamedreturns
	for n == 0 && err == nil {
		n, err = r.logs.Read(buffer)

		// return EOF if file close by context
		if errors.Is(err, fs.ErrClosed) {
			return n, io.EOF
		}

		// wait and ignore EOF until stopped
		if n == 0 && err == io.EOF && !r.doner.Done() {
			// return EOF if file close by context
			if _, err = r.inotify.Read(r.events); errors.Is(err, fs.ErrClosed) {
				return 0, io.EOF
			}
			// clear EOF and try again
			err = nil
		}
	}
	return
}

// Close releases the reader and is safe to call more than once and
// concurrently with Read.
func (r *JobReader) Close() error {
	r.closeOnce.Do(func() {
		close(r.closed)
		if err := errors.Join(r.inotify.Close(), r.logs.Close()); err != nil {
			r.closeErr = fmt.Errorf("reader close: %w", err)
		}
	})
	return r.closeErr
}
