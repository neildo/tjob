package tjob_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neildo/tjob"
)

// TestMain lets tjob re-exec this test binary as the jail for real jobs.
func TestMain(m *testing.M) {
	if err := tjob.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// requireRoot skips tests that need namespaces and cgroups.
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("requires root for namespaces and cgroups")
	}
}

// newJob returns a job with limits that fit the test environment.
// Set TJOB_MNT to a block device MAJ:MIN to also exercise io.max.
func newJob(path string, args ...string) *tjob.Job {
	job := tjob.NewJob(path, args...)
	job.MemoryMB = 64
	job.ReadBPS = 20 << 20
	job.WriteBPS = 20 << 20
	job.Mnt = os.Getenv("TJOB_MNT")
	return job
}

// readAll streams a job's logs, failing if the stream does not end on its own.
func readAll(t *testing.T, job *tjob.Job) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logs, err := job.Logs(ctx)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	defer logs.Close()

	out, err := io.ReadAll(logs)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("log stream did not end after job exit: %v", ctx.Err())
	}
	return string(out)
}

type JobMock struct {
	tjob.Doner
	done int32
}

func (m *JobMock) Done() bool {
	return atomic.LoadInt32(&m.done) == 1
}

func (m *JobMock) SetDone() bool {
	return atomic.CompareAndSwapInt32(&m.done, 0, 1)
}

func TestJobReader(t *testing.T) {
	t.Parallel()

	tmp, err := os.CreateTemp(t.TempDir(), "*")
	if err != nil {
		t.Fatalf("unexpected tmp file: %v", err)
	}
	defer func() { tmp.Close() }()

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	job := JobMock{}
	go func() {
		_, _ = tmp.WriteString("Hello")
		<-time.After(time.Second)
		_, _ = tmp.WriteString("World")

		_ = job.SetDone()
	}()
	sut, err := tjob.NewJobReader(ctx, tmp.Name(), &job)
	defer func() { sut.Close() }()

	if err != nil {
		t.Fatalf("unexpected reader: %v", err)
	}
	buffer := make([]byte, 1024)
	out := ""
	for {
		n, err := sut.Read(buffer)
		if n > 0 {
			out += string(buffer[:n])
		}
		if err != nil {
			break
		}
	}
	if out != "HelloWorld" {
		t.Errorf("expected out(%s) == HelloWorld ", out)
	}
}

func TestJobReaderCancelled(t *testing.T) {
	t.Parallel()

	tmp, err := os.CreateTemp(t.TempDir(), "*")
	if err != nil {
		t.Fatalf("unexpected tmp file: %v", err)
	}
	defer func() { tmp.Close() }()

	// mock job write to file and never finishes
	_, _ = tmp.WriteString("Hello")
	go func() {
		<-time.After(10 * time.Second)
		_, _ = tmp.WriteString("World")
	}()

	// user request logs
	ctx, cancel := context.WithCancel(context.TODO())
	sut, err := tjob.NewJobReader(ctx, tmp.Name(), &JobMock{})
	defer func() { sut.Close() }()

	if err != nil {
		t.Fatalf("unexpected reader: %v", err)
	}
	// user cancels logs
	go func() {
		<-time.After(time.Second)
		cancel()
	}()

	buffer := make([]byte, 1024)
	out := ""
	for {
		n, err := sut.Read(buffer)
		if n > 0 {
			out += string(buffer[:n])
		}
		if err != nil {
			break
		}
	}
	if out != "Hello" {
		t.Errorf("expected out(%s) == Hello", out)
	}
}

func TestJobReaderCloseStopsGoroutine(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "*")
	if err != nil {
		t.Fatalf("unexpected tmp file: %v", err)
	}
	defer tmp.Close()

	before := runtime.NumGoroutine()
	for range 10 {
		sut, err := tjob.NewJobReader(context.Background(), tmp.Name(), &JobMock{})
		if err != nil {
			t.Fatalf("unexpected reader: %v", err)
		}
		if err := sut.Close(); err != nil {
			t.Fatalf("unexpected close: %v", err)
		}
	}
	// goroutines exit asynchronously after Close
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("leaked %d goroutines after Close", n-before)
	}
}

func TestJobExitCode(t *testing.T) {
	requireRoot(t)
	t.Parallel()

	job := newJob("/bin/sh", "-c", "printf out; exit 7")
	if err := job.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if out := readAll(t, job); out != "out" {
		t.Errorf("logs = %q, want %q", out, "out")
	}
	_ = job.Wait()
	if got := job.Status().Exit; got != 7 {
		t.Errorf("exit = %d, want 7", got)
	}
}

func TestJobStopBeforeStart(t *testing.T) {
	t.Parallel()

	job := tjob.NewJob("/bin/true")
	if err := job.Stop(); !errors.Is(err, tjob.ErrNotStarted) {
		t.Errorf("stop = %v, want ErrNotStarted", err)
	}
}

func TestJobStartFailure(t *testing.T) {
	requireRoot(t)
	t.Parallel()

	job := tjob.NewJob("/bin/true")
	job.Mnt = "bogus" // rejected by io.max
	if err := job.Start(context.Background()); err == nil {
		t.Fatal("start: want error for invalid Mnt")
	}

	done := make(chan error, 1)
	go func() { done <- job.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("wait: want start error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait blocked after failed start")
	}
	if _, err := job.Logs(context.Background()); !errors.Is(err, tjob.ErrNotStarted) {
		t.Errorf("logs = %v, want ErrNotStarted", err)
	}
	if err := job.Stop(); !errors.Is(err, tjob.ErrNotStarted) {
		t.Errorf("stop = %v, want ErrNotStarted", err)
	}
	if !job.Status().Stopped() {
		t.Error("status: want stopped after failed start")
	}
}

func TestJobCgroupRemoved(t *testing.T) {
	requireRoot(t)
	t.Parallel()

	job := newJob("/bin/true")
	if err := job.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := job.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if _, err := os.Stat("/sys/fs/cgroup/" + job.Id); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cgroup still exists after exit: %v", err)
	}
}

// TestJobLogsDuringStart reads logs concurrently with Start; run with -race.
func TestJobLogsDuringStart(t *testing.T) {
	requireRoot(t)
	t.Parallel()

	job := newJob("/bin/echo", "hi")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := job.Start(context.Background()); err != nil {
			t.Errorf("start: %v", err)
		}
	}()
	for {
		logs, err := job.Logs(context.Background())
		if err == nil {
			logs.Close()
			break
		}
		if !errors.Is(err, tjob.ErrNotStarted) {
			t.Fatalf("logs: %v", err)
		}
	}
	wg.Wait()
	_ = job.Wait()
}

// TestJobLogsStreamEnds checks every reader sees EOF once its job exits.
func TestJobLogsStreamEnds(t *testing.T) {
	requireRoot(t)
	t.Parallel()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job := newJob("/bin/echo", "hi")
			if err := job.Start(context.Background()); err != nil {
				t.Errorf("start: %v", err)
				return
			}
			if out := readAll(t, job); out != "hi\n" {
				t.Errorf("logs = %q, want %q", out, "hi\n")
			}
		}()
	}
	wg.Wait()
}
