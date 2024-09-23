package tjob_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neildo/tjob"
)

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
