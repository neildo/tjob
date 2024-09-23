package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/neildo/tjob"
	"github.com/neildo/tjob/internal/proto"
	"github.com/neildo/tjob/internal/service"
)

var (
	mnt  = flag.String("mnt", "", "$MAJ:$MIN device number for mnt namespace")
	cpu  = flag.Int("cpu", 20, "max cpu percentage")
	mem  = flag.Int("mem", 20, "max memory in MB")
	rbps = flag.Int("rbps", 20*1024*1024, "max reads in bytes/sec")
	wbps = flag.Int("wbps", 20*1024*1024, "max writes in bytes/sec")
	host = flag.String("host", "localhost:8080", "server url")
	ca   = flag.String("ca", ".tjob/ca.crt", "CA cert file")
	cert = flag.String("cert", ".tjob/svc.crt", "server cert file")
	key  = flag.String("key", ".tjob/svc.key", "server key file")
)

func main() {
	// MUST init tjob before starting any job for isolation
	if err := tjob.Init(); err != nil {
		fmt.Println(err.Error())
		return
	}
	flag.Parse()

	if *mnt == "" {
		log.Fatal("-mnt required")
	}
	certs, pool, err := proto.NewCertificates(*cert, *key, *ca)
	if err != nil {
		log.Fatalf("certs: %v", err)
	}
	creds := credentials.NewTLS(&tls.Config{
		Certificates: certs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS13})

	server := grpc.NewServer(
		grpc.Creds(creds),
	)
	proto.RegisterJobServer(server, &service.JobServer{
		Mnt:        *mnt,
		CPUPercent: *cpu,
		MemoryMB:   *mem,
		ReadBPS:    *rbps,
		WriteBPS:   *wbps,
	})
	l, err := net.Listen("tcp", *host)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("listen on %s\n", *host)

	if err := server.Serve(l); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
