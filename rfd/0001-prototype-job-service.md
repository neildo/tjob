---
authors: Neil Do (neilexpress@gmail.com)
state: draft
---

# RFD 1 - Prototype Job Service

## What

Prototype job service that provides an API, CLI, and Library to run arbitrary Linux processes.

## Why

This prototype has two goals:

 * help Teleport assess how candidates reason API design, write production code, and talk through problems before solving them.

 * help candidates simulate typical work activities at Teleport.

## Details
Prototype job service contains the following components
- `tjobs` = gRPC service to manage and control arbitrary process 
- `tjob` = CLI to interact with `tjobs` over gRPC using mTLS authentication and authorization 
- `tjob` = Library to run arbitrary process with resource limits inside cgroups

### Assumptions
- Single instance of Linux 64-bit with cgroupv2 enabled

### Out of Scope
- High Availability 
- Observability 
- Recycling Jobs
- Network Access Jobs

### Authentication & Authorization
> Use mTLS authentication and verify client certificate. Set up strong set of cipher suites for TLS and good crypto setup for certificates. Do not use any other authentication protocols on top of mTLS. Use a simple authorization scheme.

For mutual authentication (mTLS), both client and server require certificates from trusted certificate authorities (CA).
Client and server certificates originate from the same self-signed CA; thereby, establishing the trusted relationship from the same source.
Client certificates must contain the common name (CN) to restrict jobs created by it and no other. Any authenticated client can run a new job. Clients validate the Subject Alt Name (SAN) matches the server DNS. 

```bash
cd .tjob

# generate CA's private key and self-signed certificate
openssl req -x509 -newkey Ed25519 -days 365 -nodes -keyout ca-key.pem -out ca-cert.pem -subj "/CN=issuer"

# generate server's key and cert
openssl req -x509 -newkey Ed25519 -nodes -keyout svc-key.pem -out svc-cert.pem -subj "/CN=tjobs" -addext "subjectAltName=DNS:localhost" -CA ca-cert.pem -CAkey ca-key.pem -days 30

# generate Alice's key and cert
openssl req -x509 -newkey Ed25519 -nodes -keyout cli-key.pem -out cli-cert.pem -subj "/CN=alice" -CA ca-cert.pem -CAkey ca-key.pem -days 30

# generate Bob's key and cert
openssl req -x509 -newkey Ed25519 -nodes -keyout bob-key.pem -out bob-cert.pem -subj "/CN=bob" -CA ca-cert.pem -CAkey ca-key.pem -days 30
```

Client and server communicate via TLS 1.3 as the minimum version. The following cipher suites are supported:

- TLS_AES_128_GCM_SHA256
- TLS_AES_256_GCM_SHA384
- TLS_CHACHA20_POLY1305_SHA256

Note: [Go does not allow configuring supported cipher suites when using TLS 1.3](https://go.dev/blog/tls-cipher-suites).

### CLI UX
---
> CLI should be able to connect to worker service and start, stop, get status, and stream output of a job.

```
tjobs
``` 
#### Description
Server for `tjob` CLI with limits per job
---
#### Options
```
-cpu
   max cpu percentage (default 20)
-mem
   max memory in MB (default 20)
-rbps
   max reads in bytes/sec (default 20MB)
-wbps
   max writes in bytes/sec (default 20MB)
-host
   server url (default localhost:8080)
-ca
   CA cert file (default ./tjob/ca-cert.pem)
-cert
   server cert file (default ./tjob/svc-cert.pem)
-key
   server key file (default ./tjob/svc-key.pem)
```

```
tjob run [OPTIONS] [COMMAND] [ARG...]
```
#### Description
Runs `COMMAND` in isolation from host

#### Options
```
-host
   server url (default localhost:8080)
-ca
   CA cert file (default ./tjob/ca-cert.pem)
-cert
   CLI cert file (default ./tjob/cli-cert.pem)
-key
   CLI key file (default ./tjob/cli-key.pem)
```

#### Examples
```bash
$ tjob run echo 'hello'
1fbb6e8a

$ tjob run sleep 1000
```
---

```
tjob stop [OPTIONS] JOB
```
#### Description
Signal `SIGKILL`
#### Options
```
-host
   server url (default localhost:8080)
-ca
   CA cert file (default ./tjob/ca-cert.pem)
-cert
   CLI cert file (default ./tjob/cli-cert.pem)
-key
   CLI key file (default ./tjob/cli-key.pem)
```
---

 ```
tjob ps [OPTIONS] JOB
```
#### Description
Show status of `JOB`
#### Options
```
-host
   server url (default localhost:8080)
-ca
   CA cert file (default ./tjob/ca-cert.pem)
-cert
   CLI cert file (default ./tjob/cli-cert.pem)
-key
   CLI key file (default ./tjob/cli-key.pem)
```
#### Examples
```bash
$ tjob ps 1fcc4f8b
JOB ID    COMMAND        CREATED     STATUS
1fcc4f8b  "echo hello"   1 min ago   Exited (0) 1 min ago
```
---

 ```
tjob logs [OPTIONS] JOB
```
#### Description
Output `STDOUT` and `STDERR` from start of `JOB` to now
#### Options
```
-host
   server url (default localhost:8080)
-ca
   CA cert file (default ./tjob/ca-cert.pem)
-cert
   CLI cert file (default ./tjob/cli-cert.pem)
-key
   CLI key file (default ./tjob/cli-key.pem)
```

### Proto Specification
> GRPC API to start/stop/get status/stream output of a running process.

```protobuf
syntax = "proto3";

option go_package = "tjob";
...

service Job {
  rpc Run(RunRequest) returns (RunResponse);
  rpc Stop(StopRequest) returns (StopResponse);
  rpc Status(StatusRequest) returns (StatusResponse);
  rpc Logs(LogsRequest) returns (stream LogsResponse);
}

message RunRequest {
  string path = 1; // path of process

  repeated string args = 2; // additional arguments
}

message RunResponse {
  option string job_id = 1;
}

message StopRequest {
  string job_id = 1;
}

message StopResponse {
}

message Status {
   string job_id = 1;
   
   string cmd = 2; // full command line

   Timestamp started = 3; // job start time in UTC
   
   int32 ranSecs = 4; // seconds ran since start
   
   option int32 exit = 5; // exit code from job

   option string error = 6; // any error from the job
}

message StatusRequest {
  string job_id = 1;
}

message StatusResponse {
   Status job = 1;
}

message LogsRequest {
   string job_id = 1;
}

message LogsResponse {
   bytes out = 1;
}
```
### Library UX
---
> Worker library with methods to start/stop/query status and get the output of a job. 
> Library should be able to stream the output of a running job. 
> Output should be from start of process execution. 
> Multiple concurrent clients should be supported.

Library starts job with resource limits, returns status, and enable streaming output for multiple concurrent clients.

```golang
package tjob

type (
   Status struct {
      Pid  int
      Cmd string 
      Started int64 // Unix ts (nanoseconds), zero if not started
      Stopped int64 // Unix ts (nanoseconds), zero if not started or running
      Ran float64 // seconds, zero if not started
      Exit int32 // exit code
      Error error // go error
   }

   Job struct {      
      // Set underlying os/exec.Cmd.Path
      Path string

      // Set underlying os/exec.Cmd.Args.
      Args []string
      
      // Set underlying os/exec.Cmd.Dir.
      Dir string

      // CPUPercent represents the quota of all cores. 
      CPUPercent int

      // MemoryMB represents the quota of memory to in Megabytes.
      MemoryMB int
      
      // ReadBPS represents the max bytes read per second by proc
      ReadBPS int
      
      // WriteBPS represents the max bytes write per second by proc
      WriteBPS int

      // log file bind to os/exec.Cmd.Stdout and os/exec.Cmd.Stderr
      logs *os.File
   }

   JobReader struct {
      job *Job
      logs io.ReadCloser
      wait time.Duration
   }
)

// NewJob creates Job for the given command path and args until Start()
func NewJob(path string, args ...string) *Job

// Start starts the command 
func (j *Job) Start() error

// Stop signal SIGKILL on the process group and idempotent.
func (j *Job) Stop() error

// Status returns the Status at any time and concurrency safe. 
func (j *Job) Status() Status

// Logs returns JobReader for polling logs until process stops
func (j *Job) Logs(period time.Duration) (io.ReadCloser, error) {
   return JobReader{job: j, logs: os.OpenFile(j.logs.Name(), os.O_RDONLY, 0660), wait: period}
}

// Read reads n bytes into buffer and return EOF only when Job stops
func (r *JobReader) Read(buffer []byte) (n int, err error) {
	n, err = r.logs.Read(buffer)
	// wait and ignore EOF until stopped
	if n == 0 && err == io.EOF && r.job.Status().Stopped == 0 {
		if n == 0 {
			<-time.After(r.wait)
		}
      err = nil
	}
	return
}
```

#### Example
```golang
package main

import (
   "github.com/neildo/tjob"

   "math"
)

// Stream polls the job logs to print them every period
func Stream(j *Job, period time.Duration, count int64) {
   buffer := make([]byte, 1024)

   logs, _ := job.Logs(period)
   defer must(reader.Close())

   // poll for logs from job
   for count > 0 {
      
      // read ignores EOF and waits if process still running
      n, err := logs.Read(buffer); 
      if err != nil {
         fmt.Println(err.Error())
         break
      } 
      if n > 0 {
         fmt.Print("%s", buffer[:n])
      }
      count-- 
   }
}

func main() {
   // create a long-running process, capture stdout and stderr
   job := tjob.NewJob("find", "/", "-name","*.txt")
   job.CPUPercent = 20
   job.MemoryMB = 20
   job.ReadBPS = 20 * 1024 * 1024
   job.WriteBPS = 20 * 1024 * 1024

   // Start job
   must(job.Start())

   // Status before Stop
   s := job.Status()
   fmt.Println("%+v", s)

   // Stop job after 10 seconds
   go func() {
      // simulate streaming for client 1
      Stream(job, 1 * time.Seconds, 10)
      must(job.Stop())
   }()

   // simulate streaming for client 2
   Stream(job, 1 * time.Seconds, math.MaxInt64)

   // Status after Stop
   s := job.Status()
   fmt.Println("%+v", s)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
```
### Resource Controls & Isolation 
---
> Add resource control for CPU, Memory and Disk IO per job using cgroups.
> Add resource isolation for using PID, mount, and networking namespaces.

Namespaces isolate processes to run independently of each other on the same machine with the following types: 

- PID: The Process ID namespace provides an independent set of process IDs (PIDs) from other namespaces. PID namespaces make the first process created as PID 1.
- MNT: Mount namespaces enables independent mount/unmount of mount points.
- NET: Network namespaces create an independent network stack for the process.
- UTS: UNIX Time-Sharing namespaces enable separate hostname and domain name the process.
- USER: User namespaces create separate set of UIDS and GIDS for the process.
- IPC: IPC namespaces isolate processes from inter-process communication between processes in different IPC namespaces.

For the scope above, Library creates new PID, MNT, NET, and cgroup namespace for arbitrary processes.

#### Control Group Interface Files
Library writes to the following cgroupv2 interafce files to limit the CPU, Memory, and Disk IO. 

```bash
# make cgroup for job_id adding new proc later
$ mkdir /sys/fs/cgroup/job_id

# enable cgroup for cpu, memory, and io
$ echo "+cpu +memory +io" > /sys/fs/cgroup/job_id/cgroup.subtree_control

# limits cpu <cpu_quota> and <cpu_period> to 20%
$ echo "20000 100000" > /sys/fs/cgroup/job_id/cpu.max

# limit memory to 20M
$ echo "20M" > /sys/fs/cgroup/job_id/memory.max

$ lsblk
NAME                      MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
loop0                       7:0    0  59.2M  1 loop /snap/core20/1977
loop1                       7:1    0 109.6M  1 loop /snap/lxd/24326
loop2                       7:2    0  46.4M  1 loop /snap/snapd/19459
loop3                       7:3    0  33.7M  1 loop /snap/snapd/21761
loop4                       7:4    0  59.8M  1 loop /snap/core20/2321
loop5                       7:5    0  77.4M  1 loop /snap/lxd/29353
sda                         8:0    0    64G  0 disk
├─sda1                      8:1    0     1G  0 part /boot/efi
├─sda2                      8:2    0     2G  0 part /boot
└─sda3                      8:3    0  60.9G  0 part
  └─ubuntu--vg-ubuntu--lv 253:0    0  30.5G  0 lvm  /
sr0                        11:0    1  1024M  0 rom

# limit both reads BPS and writes BPS to 20M. simplified max riops and wiops.
$ echo "253:0 rbps=2097152 wbps=2097152 riops=max wiops=max" > /sys/fs/cgroup/job_id/io.max

```
See kernel documentation on [Control Group v2](https://www.kernel.org/doc/Documentation/cgroup-v2.txt)

#### Proof of Concept
Library clones the current process with new PID, MNT, NET, and cgroup namespace before running the given command.

```golang
//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// tjobs <command> -> tjobs jail <command> -> <command>
func main() {
	fmt.Println(os.Args)
	switch os.Args[1] {
	case "jail":
		jail()
	default: // simulate tjobs receive request to run job
		run()
	}
}

func run() {
   // open cgroup created ahead of time
   cgroup, err := os.OpenFile("/sys/fs/cgroup/job_id", os.O_RDONLY, 0)
	must(err)
   defer must(cgroup.Close())

	// MUST clone self with new PID, MNT, NET, and cgroup namespace
	cmd := exec.Command("/proc/self/exe", append([]string{"jail"}, os.Args[1:]...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
	   CgroupFD: int(cgroup.Fd()),
      UseCgroupFD: cgroup.Fd() != nil,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())
}

func jail() {
	// MUST override the parent /proc before running command. linux unmount upon exit
	must(syscall.Mount("proc", "/proc", "proc", 0, ""))

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(cmd.Run())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

```
#### Example
```bash
# identify operating system
vagrant@vagrant:/vagrant/tjob$ uname -a
Linux vagrant 5.15.0-92-generic 102-Ubuntu SMP Wed Jan 10 09:37:39 UTC 2024 aarch64 aarch64 aarch64 GNU/Linux

# shell into root
$ sudo bash

# show running procs
root@vagrant:/vagrant/tjob# ps
    PID TTY          TIME CMD
   2914 pts/1    00:00:00 sudo
   2915 pts/1    00:00:00 bash
   2922 pts/1    00:00:00 ps

# ping localhost
root@vagrant:/vagrant/tjob# ping 127.0.0.1 -c 1
PING 127.0.0.1 (127.0.0.1) 56(84) bytes of data.
64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.038 ms

--- 127.0.0.1 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.038/0.038/0.038/0.000 ms

# jail bash with chain of procs
root@vagrant:/vagrant/tjob# go run cmd/tjobs/main.go bash
[/tmp/go-build1960547998/b001/exe/main bash]
[/proc/self/exe jail bash]

# show different chain of procs
root@vagrant:/vagrant/tjob# ps
    PID TTY          TIME CMD
      1 pts/1    00:00:00 exe
      6 pts/1    00:00:00 bash
     13 pts/1    00:00:00 ps

# show no network access
root@vagrant:/vagrant/tjob# ping 127.0.0.1
ping: connect: Network is unreachable

# exit jail
root@vagrant:/vagrant/tjob# exit
exit

# show original procs
root@vagrant:/vagrant/tjob# ps
    PID TTY          TIME CMD
   2914 pts/1    00:00:00 sudo
   2915 pts/1    00:00:00 bash
   2977 pts/1    00:00:00 ps

# ping localhost again
root@vagrant:/vagrant/tjob# ping 127.0.0.1 -c 1
PING 127.0.0.1 (127.0.0.1) 56(84) bytes of data.
64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.025 ms

--- 127.0.0.1 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss, time 0ms
rtt min/avg/max/mdev = 0.025/0.025/0.025/0.000 ms
```

[Deep into Container — Build your own container with Golang](https://dev.to/devopsvn/deep-into-container-build-your-own-container-with-golang-3f5e)

### Test Plan

#### Alice can view status and logs before and after job stopped.
```bash
# run job
$ tjob run find / -name *.txt
f8a9e533

# see job
$ tjob ps f8a9e533
JOB ID    COMMAND        CREATED     STATUS
f8a9e533  "find / ..."   3 secs ago  Up 3 secs

# see logs
$ tjob logs f8a9e533
/usr/share/go-1.22/src/cmd/go/testdata/script/mod_get_retract.txt
...
/usr/share/go-1.22/src/cmd/go/testdata/script/mod_get_major.txt
/usr/share/go-1.22/src/cmd/go/testdata/script/test_chatty_parallel_success.txt

# stop job
$ tjob stop f8a9e533
f8a9e533

# see job
$ tjob ps f8a9e533
JOB ID    COMMAND        CREATED      STATUS
f8a9e533  "find / ..."   10 secs ago  Exit (137)

# see logs
$ tjob logs f8a9e533
/usr/share/go-1.22/src/cmd/go/testdata/script/mod_get_retract.txt
...
/usr/share/go-1.22/src/cmd/go/testdata/script/mod_get_major.txt
/usr/share/go-1.22/src/cmd/go/testdata/script/test_chatty_parallel_success.txt
```
#### Alice can limit CPU
```bash
# limit CPU
$ tjob run sha1sum /dev/random
33967794
$ ps -p $(pgrep sha1sum) -o %cpu
%CPU
 20
 ```
#### Alice can limit IO
```bash
# limit IO
$ tjob run dd if=/dev/zero of=/tmp/tjob bs=512M count=1
38ee3d9c
# watch IO
$ iostat 1 -d -h -y -k -p sda
Device:            tps    kB_read/s    kB_wrtn/s    kB_read    kB_wrtn
...
```
#### Alice cannot run job with network access 
```bash
$ ping localhost -c 1
ping: connect: Network is unreachable
```
#### Alice cannot run job to see other procs
```bash
$ tjob run ps
650d452d
$ tjob logs 650d452d
    PID TTY          TIME CMD
      1 pts/1    00:00:00 exe
      3 pts/1    00:00:00 ps
```
#### Bob cannot see job status and logs created by Alice
```bash
# cannot see status above
$ tjob ps f8a9e533
$
# cannot see logs above
$ tjob logs f8a9e533
$ 
```
#### Bob cannot stop job created by Alice
```bash
# cannot stop job
$ tjob stop f8a9e533
$ 
```