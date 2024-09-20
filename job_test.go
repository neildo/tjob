package tjob_test

import (
	"context"
	"io"
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
	tmp, err := os.CreateTemp("", "*")
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
	if err != nil {
		t.Fatalf("unexpected reader: %v", err)
	}
	buffer := make([]byte, 1024)
	out := ""
	for {
		n, err := sut.Read(buffer)
		if err == io.EOF {
			break
		}
		if n > 0 {
			out += string(buffer[:n])
		}
	}
	if out != "HelloWorld" {
		t.Errorf("expected out(%s) == HelloWorld ", out)
	}
}
