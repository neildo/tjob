# tjob

> Prototype job service that provides an API, CLI, and Library to run arbitrary Linux processes.

[See RFD for more.](./rfd/0001-prototype-job-service.md) 

# Library Demo
```bash
$ git clone https://github.com/neildo/tjob

$ cd tjob

$ lsblk
NAME                      MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
loop0                       7:0    0  59.8M  1 loop /snap/core20/2321
loop1                       7:1    0  77.4M  1 loop /snap/lxd/29353
loop2                       7:2    0  33.7M  1 loop /snap/snapd/21761
loop3                       7:3    0  46.4M  1 loop /snap/snapd/19459
loop4                       7:4    0 109.6M  1 loop /snap/lxd/24326
loop5                       7:5    0  59.8M  1 loop /snap/core20/2383
sda                         8:0    0    64G  0 disk
├─sda1                      8:1    0     1G  0 part /boot/efi
├─sda2                      8:2    0     2G  0 part /boot
└─sda3                      8:3    0  60.9G  0 part
  └─ubuntu--vg-ubuntu--lv 253:0    0  30.5G  0 lvm  /
sr0                        11:0    1  1024M  0 rom

# run the demo with the MAJ:MIN device number with '/'
$ sudo go run examples/demo/main.go -mnt 253:0 find / -name *.txt
41eed6fc-e11c-4d97-89eb-11f9463659bb
[/proc/self/exe .tjob find / -name *.txt]
/var/lib/cloud/instances/iid-datasource-none/cloud-config.txt
/var/lib/cloud/instances/iid-datasource-none/user-data.txt
/var/lib/cloud/instances/iid-datasource-none/vendor-data2.txt
...
/snap/lxd/29353/lib/python3/dist-packages/certifi-2019.11.28.egg-info/dependency_links.txt
/snap/lxd/29353/lib/python3/dist-packages/certifiJobId=055c6508-d43e-4e48-b903-5b68b170f130
Status={Pid:30449 Cmd:find / -name *.txt StartedAt:2024-09-23 12:19:16.920139364 +0000 UTC m=+0.003127958 StoppedAt:2024-09-23 12:19:18.946252189 +0000 UTC m=+2.029240783 Ran:2.026112825s Exit:2 Error:force stop
exit status 2}

# run go tests
$ go test -race -shuffle on -count=1 -v .
-test.shuffle 1727099302765219893
=== RUN   TestJobReaderCancelled
=== PAUSE TestJobReaderCancelled
=== RUN   TestJobReader
=== PAUSE TestJobReader
=== CONT  TestJobReader
=== CONT  TestJobReaderCancelled
--- PASS: TestJobReaderCancelled (1.01s)
--- PASS: TestJobReader (1.01s)
PASS
ok  	github.com/neildo/tjob	2.023s
```
[See example code for more.](./examples/demo/main.go) 

# Build `tjobs` API & `tjob` CLI
```bash
# build the certs, API, and CLI
$ make build
generate self-signed certs for CA, API, and CLIs under .tjob/
-----
-----
-----
-----
build API and CLI
total 27M
drwxr-xr-x 1 vagrant vagrant 384 Jan  1  1970 .
drwxr-xr-x 1 vagrant vagrant 576 Sep 23 11:00 ..
-rw-r--r-- 1 vagrant vagrant 477 Sep 23 11:02 other.crt
-rw------- 1 vagrant vagrant 119 Sep 23 11:02 other.key
-rw-r--r-- 1 vagrant vagrant 481 Sep 23 11:02 ca.crt
-rw------- 1 vagrant vagrant 119 Sep 23 11:02 ca.key
-rw-r--r-- 1 vagrant vagrant 481 Sep 23 11:02 cli.crt
-rw------- 1 vagrant vagrant 119 Sep 23 11:02 cli.key
-rw-r--r-- 1 vagrant vagrant 509 Sep 23 11:02 svc.crt
-rw------- 1 vagrant vagrant 119 Sep 23 11:02 svc.key
-rwxr-xr-x 1 vagrant vagrant 14M Sep 23 11:02 tjob
-rwxr-xr-x 1 vagrant vagrant 14M Sep 23 11:02 tjobs
```

# Run `tjobs` API
```bash
# show help for tjobs API 
$ .tjob/tjobs -h
Usage of .tjob/tjobs:
  -ca string
    	CA cert file (default ".tjob/ca.crt")
  -cert string
    	server cert file (default ".tjob/svc.crt")
  -cpu int
    	max cpu percentage (default 20)
  -host string
    	server url (default "localhost:8080")
  -key string
    	server key file (default ".tjob/svc.key")
  -mem int
    	max memory in MB (default 20)
  -mnt string
    	MAJ:MIN device number for mnt namespace
  -rbps int
    	max reads in bytes/sec (default 20971520)
  -wbps int
    	max writes in bytes/sec (default 20971520)

# running tjobs API requires MAJ:MIN device number of '/'
$ .tjob/tjobs
2024/09/23 11:04:04 -mnt required

# show available MAJ:MIN device numbers
$ lsblk
NAME                      MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
loop0                       7:0    0  59.8M  1 loop /snap/core20/2321
loop1                       7:1    0  77.4M  1 loop /snap/lxd/29353
loop2                       7:2    0  33.7M  1 loop /snap/snapd/21761
loop3                       7:3    0  46.4M  1 loop /snap/snapd/19459
loop4                       7:4    0 109.6M  1 loop /snap/lxd/24326
loop5                       7:5    0  59.8M  1 loop /snap/core20/2383
sda                         8:0    0    64G  0 disk
├─sda1                      8:1    0     1G  0 part /boot/efi
├─sda2                      8:2    0     2G  0 part /boot
└─sda3                      8:3    0  60.9G  0 part
  └─ubuntu--vg-ubuntu--lv 253:0    0  30.5G  0 lvm  /
sr0                        11:0    1  1024M  0 rom

# MUST run `sudo tjobs` for resource isolation
$ sudo .tjob/tjobs -mnt 253:0
2024/09/23 11:04:41 listen on localhost:8080
```
# Run `tjob` CLI
```bash
# show help for tjob CLI
$ .tjob/tjob -h
Usage .tjob/tjob COMMAND

Commands:
  run	[OPTIONS] COMMAND [ARG...]
  stop	[OPTIONS] JOB
  ps	[OPTIONS] JOB
  logs	[OPTIONS] JOB

Options:
  -ca string
    	CA cert file (default ".tjob/ca.crt")
  -cert string
    	cli cert file (default ".tjob/cli.crt")
  -host string
    	server url (default "localhost:8080")
  -key string
    	cli key file (default ".tjob/cli.key")

# run job to find all text files
$ .tjob/tjob run find / -name *.txt
32d0fe6e

# show the status of the running job
$ .tjob/tjob ps 32d0fe6e
JOB ID    COMMAND                CREATED     STATUS
32d0fe6e  "find / -name *.txt  " 20s         20s

# stream the logs of the job and Ctrl+C to cancel it
$ .tjob/tjob logs 32d0fe6e
/var/lib/cloud/instances/iid-datasource-none/cloud-config.txt
/var/lib/cloud/instances/iid-datasource-none/user-data.txt
/var/lib/cloud/instances/iid-datasource-none/vendor-data2.txt
...
/usr/share/go-1.22/src/cmd/go/testdata/mod/example.com_retract_newergoversion_v1.2.0.txt
/usr/share/go-1.22/src/cmd/go/testdata/mod/rsc.io_!c!g!o_v1.0.0.txt
/usr/share/go-1.22/src/cmd/go/testdata/mod/^C

# show the status of it again
$ .tjob/tjob ps 32d0fe6e
JOB ID    COMMAND                CREATED     STATUS
32d0fe6e  "find / -name *.txt  " 38s         38s

# stop the running job
$ .tjob/tjob stop 32d0fe6e
32d0fe6e

# show the status after stopped
$ .tjob/tjob ps 32d0fe6e
JOB ID    COMMAND                CREATED     STATUS
d824c304  "find / -name *.txt  " 53s         Exit (2) force stop;exit status 2

# show the logs after stopped
$ .tjob/tjob logs 32d0fe6e
/var/lib/cloud/instances/iid-datasource-none/cloud-config.txt
/var/lib/cloud/instances/iid-datasource-none/user-data.txt
/var/lib/cloud/instances/iid-datasource-none/vendor-data2.txt
...
/usr/share/go-1.22/src/cmd/go/testdata/mod/example.com_retract_newergoversion_v1.2.0.txt
/usr/share/go-1.22/src/cmd/go/testdata/mod/rsc.io_!c!g!o_v1.0.0.txt
/usr/share/go-1.22/src/cmd/go/testdata/mod/$

# run short-lived job
$ .tjob/tjob run uname -a
9ac4f767

# show the status of it
$ .tjob/tjob ps 9ac4f767
JOB ID    COMMAND                CREATED     STATUS
9ac4f767  "uname -a            " 14s         Exit (0)

# show the logs of it
$ .tjob/tjob logs 9ac4f767
Linux vagrant 5.15.0-92-generic 102-Ubuntu SMP Wed Jan 10 09:37:39 UTC 2024 aarch64 aarch64 aarch64 GNU/Linux

# others cannot see logs
$ .tjob/tjob logs -cert .tjob/other.crt -key .tjob/other.key 9ac4f767
2024/09/23 12:11:35 rpc error: code = Unknown desc = unauthorized

# others cannot see status
$ .tjob/tjob ps -cert .tjob/other.crt -key .tjob/other.key 9ac4f767
2024/09/23 12:11:58 rpc error: code = Unknown desc = unauthorized

# others cannot stop it
$ .tjob/tjob stop -cert .tjob/other.crt -key .tjob/other.key 9ac4f767
2024/09/23 12:12:22 rpc error: code = Unknown desc = unauthorized

# others can run own job
$ .tjob/tjob run -cert .tjob/other.crt -key .tjob/other.key uname -a
637cb2a0

# others can see status of it
$ .tjob/tjob ps -cert .tjob/other.crt -key .tjob/other.key 637cb2a0
JOB ID    COMMAND                CREATED     STATUS
637cb2a0  "uname -a            " 47s         Exit (0)

# others can see logs of it
$ .tjob/tjob logs -cert .tjob/other.crt -key .tjob/other.key 637cb2a0
Linux vagrant 5.15.0-92-generic 102-Ubuntu SMP Wed Jan 10 09:37:39 UTC 2024 aarch64 aarch64 aarch64 GNU/Linux
```