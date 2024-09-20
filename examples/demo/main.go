package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/neildo/tjob"
)

var (
	mnt  = flag.String("mnt", "", "$MAJ:$MIN device number")
	cpu  = 20
	mem  = 10
	rbps = 1024
	wbps = 1024
	wait = 120
)

func main() {
	// MUST init tjob before starting any job for isolation
	if err := tjob.Init(); err != nil {
		fmt.Println(err.Error())
		return
	}
	flag.Parse()

	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <command>", os.Args[0])
	}
	args := os.Args
	if runtime.GOOS == "linux" {
		if *mnt == "" {
			log.Fatalf("Usage: %s -mnt <$MAJ:$MIN> <command>", os.Args[0])
		}
		args = os.Args[3:]
	} else {
		args = os.Args[1:]
	}

	// create new job with resource limits
	job := tjob.NewJob(args[0], args[1:]...)
	job.Mnt = *mnt
	job.CPUPercent = cpu
	job.MemoryMB = mem
	job.ReadBPS = rbps
	job.WriteBPS = wbps

	// Start job with fail safe timeout
	c, cancel := context.WithTimeout(context.TODO(), time.Hour)
	defer cancel()
	if err := job.Start(c); err != nil {
		log.Fatal(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Stop job after 10 seconds
	go func() {
		defer wg.Done()

		ctx, cancel := context.WithTimeout(context.TODO(), time.Duration(wait)*time.Second)
		defer cancel()

		buffer := make([]byte, 1024)
		logs, _ := job.Logs(ctx)
		defer func() { logs.Close() }()

		// poll for logs from job
		for {
			n, err := logs.Read(buffer)
			if n > 0 {
				fmt.Printf("%s", string(buffer[:n]))
			}
			if err == io.EOF {
				break
			}
		}

		if err := job.Stop(); err != nil {
			log.Fatal(err)
		}
	}()

	// wait for job to stop
	if err := job.Wait(); err != nil {
		fmt.Printf("JobId=%s\nStatus=%+v\n", job.Id, job.Status())
	}
	wg.Wait()
}
