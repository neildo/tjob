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

 * help candidates simulate the typical work activities at Teleport.

## Details
Prototype job service contains the following components
- tjobs = gRPC service to manage and control arbitrary process execution as job
- tjob = CLI to interact with tjobs service over gRPC using mTLS authentication and authorization 
- tjob = Library to run arbitrary process with resource limits inside cgroups
- jail = Sandbox to isolate mounts from arbitrary process

### Assumptions
- Support single instance of Linux 64-bit with cgroupv2 enabled

### Out of Scope
- High Availability 
- Observability 
- Recycling Jobs

### Authentication & Authorization
> Use mTLS authentication and verify client certificate. Set up strong set of cipher suites for TLS and good crypto setup for certificates. Do not use any other authentication protocols on top of mTLS.
Use a simple authorization scheme.

For mutual authentication (mTLS), both client and server requires certificates from trusted certificate authority (CA). Instead of well-known CA like Verisign, OpenSSL can generate the RSA 256 certificates requried for the CA, clients, and servers for the **scope of this prototype.**

The client certificate must contain the common name (CN) to restrict jobs created by it and no other clients. Any authenticated client can run a new job. 

### CLI UX
---
> CLI should be able to connect to worker service and start, stop, get status, and stream output of a job.

```
tjobs
``` 
#### Description
Server for `tjob` CLI with hardcoded value for prototype
- localhost:8080
- ca = .tjob/ca.crt
- cert = .tjob/svc.crt
- key = .tjob/svc.key
- max cpu percentage = 20
- max memory in MB = 20MB
- max reads in bytes/sec = 20MB
- max writes in bytes/sec = 20MB
---

```
tjob run [OPTIONS] [COMMAND] [ARG...]
```
#### Description
Runs `COMMAND` in isolation from host

#### Options
```
-ca
   CA cert file (default ./tjob/ca.crt)
-cert
   CLI cert file (default ./tjob/cli.crt)
-key
   CLI key file (default ./tjob/cli.key)
```

#### Examples
```bash
$ tjob run echo 'hello'
1fbb6e8a-788c-45bf-9996-1b45bb6a99d0

$ tjob run sleep 1000
```
---

```
tjob stop [OPTIONS] JOB
```
#### Description
Signal `SIGTERM` first and `SIGKILL` after grace period
#### Options
```
-ca
   CA cert file (default ./tjob/ca.crt)
-cert
   CLI cert file (default ./tjob/cli.crt)
-key
   CLI key file (default ./tjob/cli.key)
```
---

 ```
tjob ps [OPTIONS] 
```
#### Description
Show running jobs 
#### Options
```
-a
   show all jobs 
-ca
   CA cert file (default ./tjob/ca.crt)
-cert
   CLI cert file (default ./tjob/cli.crt)
-key
   CLI key file (default ./tjob/cli.key)
```
#### Examples
```bash
$ tjob ps
JOB ID    COMMAND        CREATED     STATUS
1fcc4f8b  "echo hello"   1 min ago   Exited (0) 1 min ago
1abb6e8a  "sleep 1000"   3 secs ago  Up 3 secs    
```
---

 ```
tjob logs [OPTIONS] JOB
```
#### Description
Output `STDOUT` and `STDERR` from start of `JOB` to now
#### Options
```
-ca
   CA cert file (default ./tjob/ca.crt)
-cert
   CLI cert file (default ./tjob/cli.crt)
-key
   CLI key file (default ./tjob/cli.key)
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
  rpc Logs(LogsRequest) returns (LogsResponse);
}

message RunRequest {
  string path = 1;
  repeated string args = 2;
}

message RunResponse {
  option string jobId = 1;
  option string error = 2;
}

message StopRequest {
  string jobId = 1;
}

message StopResponse {
   option string error = 2;
}

message Status {
   string jobId = 1;
   string cmd = 2;
   Timestamp started = 3;
   Timestamp stoped = 4;
   int32 elapseSecs = 5;
   option int64 exit = 6;
   option string error = 7;
}

message StatusRequest {
  bool all = 1;
}

message StatusResponse {
   repeated Status jobs = 1;
   option string error = 2;
}

message LogsRequest {
   string jobId = 1;
}

message LogsResponse {
   repeated string logs = 1;
}

```
### Library
> Worker library with methods to start/stop/query status and get the output of a job. 
> Library should be able to stream the output of a running job. 
> Output should be from start of process execution. 
> Multiple concurrent clients should be supported.

```golang
package tjob

type (
   Status struct {
      Pid  int
      Cmd string 
      Started int64 // Unix ts (nanoseconds), zero if not started
      Stopped int64 // Unix ts (nanoseconds), zero if not started or running
      Elapsed float64 // seconds, zero if not started
      Exit int64 // exit code
      Error error // go error
      Lines []string // combined stdout & stderr
   }

   Job struct {      
      *sync.Mutex

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
      
      statusChan  chan Status // nil until Run() called
      stdout      *LineBuffer 
   }

   // Unbounded buffer for command output and safe for multiple goroutines to read by calling Lines()
   LineBuffer struct {
      *sync.Mutex

      buf   *bytes.Buffer
      lines []string
   }
)

// NewJob creates Job for the given command path and args without starting until called Run()
func NewJob(path string, args ...string) *Job

// Run starts the command and returns a channel that callers can receive the final Status when done
func (j *Job) Run() <-chan Status

// Stop signals SIGTERM on the process group and idempotent.
func (j *Job) Stop() error

// Status returns the Status at any time and concurrently safe for multiple goroutines. 
func (j *Job) Status() Status

// Used directly with Go standard library os/exec.Command as io.Writer. 
func NewLineBuffer()  *LineBuffer

// Write makes LineBuffer implement the io.Writer interface
func (l *LineBuffer) Write(p []byte) (n int, err error)

// Lines returns lines of output from the Cmd
func (l *LineBuffer) Lines() []string

```

#### TODO: Example
```golang
package main

import (
   "github.com/neildo/tjob"
)

func main() {
   // Start a long-running process, capture stdout and stderr
	job := tjob.NewJob("ping", "127.0.0.1")

   // TODO: Add resource limits
	statusChan := job.Run() // non-blocking

	// Stop command after 2 seconds
	go func() {
		<-time.After(10 * time.Seconds)
		job.Stop()
	}()

	// Check if command is done
	select {
	case finalStatus := <-statusChan:
		// done
	default:
		// no, still running
	}

   s := job.Status()
   n := len(s.Lines)
   fmt.Println(s.Lines[n-1])
}

```
### Resource Control
> Add resource control for CPU, Memory and Disk IO per job using cgroups.

#### Proof of Concept
```bash
# target Ubuntu 24 ARM
$ uname -a
Linux vagrant 5.15.0-92-generic 102-Ubuntu SMP Wed Jan 10 09:37:39 UTC 2024 aarch64 aarch64 aarch64 GNU/Linux

# make cgroup for jobId
$ mkdir /sys/fs/cgroup/jobId

# enable cgroup for cpu, memory, and io
$ echo "+cpu +memory +io" > /sys/fs/cgroup/jobId/cgroup.subtree_control

# limits cpu <cpu_quota> and <cpu_period> to 20%
$ echo "20000 100000" > /sys/fs/cgroup/jobId/cpu.max

# limit memory to 20M
$ echo "20M" > /sys/fs/cgroup/jobId/memory.max

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

# limit both reads BPS and writes BPS to 2M
$ echo "8:0 rbps=2097152 wbps=2097152 riops=max wiops=max" > /sys/fs/cgroup/jobId/io.max

```
See kernel documentation on [Control Group v2](https://www.kernel.org/doc/Documentation/cgroup-v2.txt)

### Resource Isolation
> Add resource isolation for using PID, mount, and networking namespaces.

#### TODO: Proof of Concept

```golang
package main

import (
    "os"
    "os/exec"
    "syscall"
)

// ./jail run <command>
func main() {
    switch os.Args[1] {
    case "run":
        run()
    case "child":
        child()
    default:
        panic("Error")
    }
}

func run() {
    cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
    // TODO: How can I capture stdout and stderr?
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.SysProcAttr = &syscall.SysProcAttr{
      Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
      Unshareflags: syscall.CLONE_NEWNS,
		// TODO: UseCgroupFD: true,
    }

    must(cmd.Run())
}

func child() {
    syscall.Sethostname([]byte("job"))
    // change the filesystem
    must(syscall.Chdir("/"))
    must(syscall.Mount("proc", "proc", "proc", 0, ""))

    cmd := exec.Command(os.Args[2], os.Args[3:]...)
    // TODO: How can I capture stdout and stderr?
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
[Deep into Container — Build your own container with Golang](https://dev.to/devopsvn/deep-into-container-build-your-own-container-with-golang-3f5e)

### TODO: Test Plan
```
TODO

```