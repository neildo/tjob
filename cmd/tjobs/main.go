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
	// MUST clone self with new PID and networking namespace before mount namespace
	cmd := exec.Command("/proc/self/exe", append([]string{"jail"}, os.Args[1:]...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
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
