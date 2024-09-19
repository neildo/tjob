package tjob

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/google/uuid"
)

const (
	cgroupRoot     = "/sys/fs/cgroup/tjobs"
	cpuPeriod      = 100000
	cgroupFileMode = 0o500
)

// NewJob creates Job for the given command path and args until Start()
func NewJob(path string, args ...string) *Job {
	s := Status{
		Cmd:  path,
		Exit: -1,
	}
	return &Job{
		Id:       uuid.New().String(),
		jailPath: "/proc/self/exe",
		Path:     path,
		Args:     args,
		status:   s,
		doneCh:   make(chan struct{}),
	}
}

func mount() error {
	// MUST override the parent /proc before running command. linux unmount upon exit
	return syscall.Mount("proc", "/proc", "proc", 0, "")
}

// jail creates the namespaces required by the job to isolate exec.Cmd

func jail(ctx context.Context, j *Job) (*exec.Cmd, error) {
	cgroupJob := fmt.Sprintf("%s/%s", cgroupRoot, j.Id)

	// create a directory structure like /sys/fs/cgroup/tjobs/<job_id>
	if err := os.MkdirAll(cgroupJob, cgroupFileMode); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cgroupJob, err)
	}

	// enable cpu, io, and memory controllers
	path := cgroupRoot + "/cgroup.subtree_control"
	if err := os.WriteFile(path, []byte("+cpu +io +memory"), cgroupFileMode); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// limit cpu
	if j.CPUPercent > 0 {
		n := float32(j.CPUPercent) / 100 * cpuPeriod
		content := fmt.Sprintf("%d %d", int(n), cpuPeriod)
		path = cgroupJob + "/cpu.max"
		if err := os.WriteFile(path, []byte(content), cgroupFileMode); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}

	// limit memory
	path = cgroupJob + "/memory.max"
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%dM", j.MemoryMB)), cgroupFileMode); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// limit rbps and wbps
	content := fmt.Sprintf("%s rbps=%d wbps=%d riops=max wiops=max", j.Mnt, j.ReadBPS, j.WriteBPS)
	path = cgroupJob + "/io.max"
	if err := os.WriteFile(path, []byte(content), cgroupFileMode); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// open cgroup file to jail clone
	cgroup, err := os.OpenFile(cgroupJob, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cgroupJob, err)
	}

	args := append([]string{JailOp, j.Path}, j.Args...)
	cmd := exec.CommandContext(ctx, j.jailPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
		CgroupFD:     int(cgroup.Fd()),
		UseCgroupFD:  true,
	}
	j.cgroup = cgroup

	return cmd, nil
}

// NewJobReader returns the io.ReadCloser
func NewJobReader(ctx context.Context, j *Job) (io.ReadCloser, error) {
	l, err := os.OpenFile(j.logs.Name(), os.O_RDONLY, 0660)
	if err != nil {
		return nil, err
	}
	// reader specific notify
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		defer l.Close()
		return nil, os.NewSyscallError("inotify_init", err)
	}
	// close file to unblock reads if context is done
	file := os.NewFile(uintptr(fd), l.Name())
	go func() {
		<-ctx.Done()
		file.Close()
	}()
	return &JobReader{job: j, logs: l, inotify: file}, nil
}

// Read reads n bytes into buffer and return EOF only when Job stops
func (r *JobReader) Read(buffer []byte) (n int, err error) {
	n, err = r.logs.Read(buffer)
	// wait and ignore EOF until stopped
	if n == 0 && err == io.EOF && !r.job.Status().Stopped() {

		if n == 0 {
			// watch for writes and close event
			wd, err := syscall.InotifyAddWatch(int(r.inotify.Fd()), r.job.logs.Name(), syscall.IN_MODIFY|syscall.IN_CLOSE)
			if err != nil {
				return 0, os.NewSyscallError("inotify_add_watch", err)
			}
			// TODO: verify remove watch clears out inotify events
			defer syscall.InotifyRmWatch(int(r.inotify.Fd()), uint32(wd))

			// block until closed or write event
			b := make([]byte, syscall.SizeofInotifyEvent*128)
			if _, err = r.inotify.Read(b); err != nil {
				return 0, err
			}
		}
		err = nil
	}
	return
}

func (r *JobReader) Close() error {
	r.inotify.Close()
	return r.logs.Close()
}
