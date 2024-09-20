package tjob_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/neildo/tjob"
)

type JobMock struct {
	tjob.Doner
	done bool
}

func (m *JobMock) Done() bool {
	return m.done
}

func TestJobReader(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "*")
	if err != nil {
		t.Fatalf("unexpected tmp file: %v", err)
	}
	defer func() { tmp.Close() }()

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	m := JobMock{}
	go func() {
		tmp.Write([]byte("Hello"))
		<-time.After(time.Second)
		tmp.Write([]byte("World"))
		m.done = true
	}()
	sut, err := tjob.NewJobReader(ctx, tmp.Name(), &m)
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
	tmp, err := os.CreateTemp(t.TempDir(), "*")
	if err != nil {
		t.Fatalf("unexpected tmp file: %v", err)
	}
	defer func() { tmp.Close() }()

	// mock job write to file and never finishes
	m := JobMock{}
	tmp.Write([]byte("Hello"))
	go func() {
		<-time.After(10 * time.Second)
		tmp.Write([]byte("World"))
	}()

	// user request logs
	ctx, cancel := context.WithCancel(context.TODO())
	sut, err := tjob.NewJobReader(ctx, tmp.Name(), &m)
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
