//go:build !linux

package tjob

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

// ErrUnsupported is returned on platforms without namespaces and cgroup v2.
var ErrUnsupported = errors.New("tjob requires linux with cgroup v2")

// NewJob creates Job for the given command path and args until Start()
func NewJob(path string, args ...string) *Job {
	status := Status{
		Cmd: strings.Join(append([]string{path}, args...), " "),
	}
	return &Job{
		Id:     uuid.New().String(),
		Path:   path,
		Args:   args,
		status: status,
		doneCh: make(chan struct{}),
	}
}

func mount() error {
	return ErrUnsupported
}

func jail(context.Context, *Job) (*exec.Cmd, *os.File, error) {
	return nil, nil, ErrUnsupported
}

func removeCgroup(*os.File) error {
	return nil
}

// NewJobReader returns the io.ReadCloser
func NewJobReader(context.Context, string, Doner) (io.ReadCloser, error) {
	return nil, ErrUnsupported
}
