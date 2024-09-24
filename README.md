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
$ go test -v . -count=1
=== RUN   TestJobReader
=== PAUSE TestJobReader
=== RUN   TestJobReaderCancelled
=== PAUSE TestJobReaderCancelled
=== CONT  TestJobReader
=== CONT  TestJobReaderCancelled
--- PASS: TestJobReader (1.01s)
--- PASS: TestJobReaderCancelled (1.01s)
PASS
ok  	github.com/neildo/tjob	1.021s
```
[See example code for more.](./examples/demo/main.go) 
