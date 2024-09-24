package tjob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
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
		doneCh:   make(chan bool),
	}
}

func mount() error {
	// MUST override the parent /proc before running command. linux unmount upon exit
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("mount: %w", err)
	}
	return nil
}

// jail creates the namespaces required by the job to isolate exec.Cmd
func jail(ctx context.Context, job *Job) (*exec.Cmd, error) {
	cgroupJob := fmt.Sprintf("%s/%s", cgroupRoot, job.Id)
	cgroupJail := fmt.Sprintf("%s/jail", cgroupJob) //nolint:perfsprint

	// create a directory structure like /sys/fs/cgroup/<job_id>/jail
	if err := os.MkdirAll(cgroupJail, cgroupFileMode); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cgroupJob, err)
	}
	// remove dir if failed
	defer func() {
		if job.cgroup == nil {
			_ = unix.Rmdir(cgroupJob)
		}
	}()

	// enable cpu, io, and memory controllers
	path := cgroupRoot + "/cgroup.subtree_control"
	if err := os.WriteFile(path, []byte("+cpu +io +memory"), cgroupFileMode); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// limit cpu
	if job.CPUPercent > 0 {
		n := float32(job.CPUPercent) / 100 * cpuPeriod
		content := fmt.Sprintf("%d %d", int(n), cpuPeriod)
		path = cgroupJob + "/cpu.max"
		if err := os.WriteFile(path, []byte(content), cgroupFileMode); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}

	// limit memory
	path = cgroupJob + "/memory.max"
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%dM", job.MemoryMB)), cgroupFileMode); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// limit rbps and wbps
	content := fmt.Sprintf("%s rbps=%d wbps=%d riops=max wiops=max", job.Mnt, job.ReadBPS, job.WriteBPS)
	path = cgroupJob + "/io.max"
	if err := os.WriteFile(path, []byte(content), cgroupFileMode); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// open cgroup file to jail clone
	cgroup, err := os.OpenFile(cgroupJob, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cgroupJob, err)
	}

	args := append([]string{jailOp, job.Path}, job.Args...)
	cmd := exec.CommandContext(ctx, job.jailPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
		CgroupFD:     int(cgroup.Fd()),
		UseCgroupFD:  true,
	}
	job.cgroup = cgroup

	return cmd, nil
}

// NewJobReader returns the io.ReadCloser
func NewJobReader(ctx context.Context, filename string, doner Doner) (io.ReadCloser, error) {
	log, err := os.OpenFile(filename, os.O_RDONLY, 0o660)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	// reader specific notify
	desc, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		defer log.Close()
		return nil, fmt.Errorf("inotify_init1: %w", err)
	}
	// watch for writes and close event
	_, err = syscall.InotifyAddWatch(desc, log.Name(), syscall.IN_MODIFY|syscall.IN_CLOSE)
	if err != nil {
		return nil, fmt.Errorf("inotify_add_watch: %w", err)
	}
	// close file to unblock reads if context is done
	file := os.NewFile(uintptr(desc), log.Name())
	go func() {
		<-ctx.Done()
		file.Close()
		log.Close()
	}()
	return &JobReader{doner: doner, logs: log, inotify: file}, nil
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
			// sufficiently size buffer for events
			b := make([]byte, syscall.SizeofInotifyEvent*syscall.NAME_MAX+1)

			// return EOF if file close by context
			if _, err = r.inotify.Read(b); errors.Is(err, fs.ErrClosed) {
				return 0, io.EOF
			}
			// clear EOF and try again
			err = nil
		}
	}
	return
}

func (r *JobReader) Close() error {
	_ = r.inotify.Close()
	if err := r.logs.Close(); err != nil {
		return fmt.Errorf("reader close: %w", err)
	}
	return nil
}
