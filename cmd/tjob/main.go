package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/neildo/tjob/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	subcommands = "run stop ps logs"
	cmdSize     = 20
)

const (
	help = `
Commands:
  run	[OPTIONS] COMMAND [ARG...]
  stop	[OPTIONS] JOB
  ps	[OPTIONS] JOB
  logs	[OPTIONS] JOB

Options:`
)

func usage() {
	fmt.Printf("Usage %s COMMAND\n", os.Args[0])
	fmt.Println(help)
	flag.PrintDefaults()
}

func main() {
	var (
		host = flag.String("host", "localhost:8080", "server url")
		ca   = flag.String("ca", ".tjob/ca.crt", "CA cert file") //nolint:varnamelen
		cert = flag.String("cert", ".tjob/cli.crt", "cli cert file")
		key  = flag.String("key", ".tjob/cli.key", "cli key file")
	)
	args := os.Args
	cmd := ""
	if len(args) > 1 && strings.Contains(subcommands, os.Args[1]) {
		cmd = args[1]
		args = args[2:]
		if strings.HasPrefix(args[0], "-") {
			_ = flag.CommandLine.Parse(args)
		}
		// skip over any flags
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			args = args[2:]
		}
	}
	if len(args) == 0 || cmd == "" {
		usage()
		return
	}

	certs, pool, err := proto.NewCertificates(*cert, *key, *ca)
	if err != nil {
		log.Fatalf("certs: %v", err)
	}
	creds := credentials.NewTLS(&tls.Config{
		Certificates: certs,
		RootCAs:      pool,
	})

	conn, err := grpc.NewClient(*host, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("host: %v", err)
	}
	defer conn.Close()

	client := proto.NewJobClient(conn)
	ctx := context.TODO()

	switch cmd {
	case "run":
		r, err := client.Run(ctx, &proto.RunRequest{Path: args[0], Args: args[1:]})
		if err != nil {
			log.Fatalln(err.Error()) //nolint:gocritic
		}
		fmt.Println(r.GetJobId())
	case "stop":
		id := args[0]
		_, err := client.Stop(ctx, &proto.StopRequest{JobId: id})
		if err != nil {
			log.Fatalln(err.Error())
		}
		fmt.Println(id)
	case "ps":
		id := args[0]
		resp, err := client.Status(ctx, &proto.StatusRequest{JobId: id})
		if err != nil {
			log.Fatalln(err.Error())
		} else if resp.GetJob() == nil {
			log.Fatalln("no status")
		}
		job := resp.GetJob()

		fmt.Printf("%-10s%-20s   %-12s%-10s\n", "JOB ID", "COMMAND", "CREATED", "STATUS")
		created := time.Since(job.GetStartedAt().AsTime()).Truncate(time.Second)
		status := job.GetRan().AsDuration().Truncate(time.Second).String()
		if job.Exit != nil {
			status = fmt.Sprintf("Exit (%d) %s", job.GetExit(), strings.ReplaceAll(job.GetError(), "\n", ";"))
		}
		n := min(len(job.GetCmd()), cmdSize)
		fmt.Printf("%-10s\"%-20s\" %-12s%-10s\n", job.GetJobId(), job.GetCmd()[:n], created, status)
	case "logs":
		id := args[0]

		logs, err := client.Logs(ctx, &proto.LogsRequest{JobId: id})
		if err != nil {
			log.Fatalln(err.Error())
		}
		for {
			out, err := logs.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				log.Fatalln(err.Error())
			}
			fmt.Print(string(out.GetOut()))
		}

	default:
		usage()
	}
}
