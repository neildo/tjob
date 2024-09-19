package tjob

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
)

// NewJob creates Job for the given command path and args until Start()
func NewJob(path string, args ...string) *Job {
	s := Status{
		Cmd:  path,
		Exit: -1,
	}
	// workaround no "/proc/self/exe" on darwin
	workaround := os.Args[0]
	return &Job{
		Id:       uuid.New().String(),
		jailPath: workaround,
		Path:     path,
		Args:     args,
		status:   s,
		doneCh:   make(chan struct{}),
	}
}

// mount mocks the func on darwin
func mount() error { return nil }

// jail mocks the func on darwin
func jail(ctx context.Context, j *Job) (*exec.Cmd, error) {
	args := append([]string{JailOp, j.Path}, j.Args...)

	// mock running jail on darwin
	cmd := exec.CommandContext(ctx, j.jailPath, args...)
	return cmd, nil
}

func NewJobReader(ctx context.Context, j *Job) (io.ReadCloser, error) {
	l, err := os.OpenFile(j.logs.Name(), os.O_RDONLY, 0660)
	if err != nil {
		return nil, err
	}
	return &JobReader{job: j, logs: l}, nil
}

// Read reads n bytes into buffer and return EOF only when Job stops
func (r *JobReader) Read(buffer []byte) (n int, err error) {
	n, err = r.logs.Read(buffer)
	// wait and ignore EOF until stopped
	if n == 0 && err == io.EOF && !r.job.Status().Stopped() {
		if n == 0 {
			<-time.After(200 * time.Millisecond)
		}
		err = nil
	}
	return
}

func (r *JobReader) Close() error {
	return r.logs.Close()
}
