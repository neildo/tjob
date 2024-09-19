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
	cpu  = flag.Int("cpu", 10, "limit CPU %")
	mem  = flag.Int("mem", 10, "limit Memory in MB")
	rbps = flag.Int("rbps", 1024, "limit reads in bytes per second")
	wbps = flag.Int("wbps", 1024, "limit writes in bytes per second")
	wait = flag.Int("wait", 3, "wait time in seconds")
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
	job.CPUPercent = *cpu
	job.MemoryMB = *mem
	job.ReadBPS = *rbps
	job.WriteBPS = *wbps

	fmt.Println(job.Id)

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

		ctx, cancel := context.WithTimeout(context.TODO(), time.Duration(*wait)*time.Second)
		defer cancel()

		buffer := make([]byte, 1024)
		logs, _ := job.Logs(ctx)
		defer func() { logs.Close() }()

		// poll for logs from job
		for {
			// read ignores EOF and waits if process still running
			n, err := logs.Read(buffer)
			if err != nil {
				if err != io.EOF {
					fmt.Println(err.Error())
				}
				break
			}
			if n > 0 {
				fmt.Printf("%s", string(buffer[:n]))
			}
		}

		if err := job.Stop(); err != nil {
			log.Fatal(err)
		}
	}()

	// wait for job to stop
	if err := job.Wait(); err != nil {
		fmt.Printf("%+v\n", job.Status())
	}
	wg.Wait()
}
