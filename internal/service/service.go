package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/neildo/tjob"
	"github.com/neildo/tjob/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrUnexpected   = errors.New("unexpected")

	ErrNoPeer             = errors.New("no peer")
	ErrNoTLSInfo          = errors.New("no TLS info")
	ErrNoPeerCertificates = errors.New("no peer certificates")
)

type userJob struct {
	user string
	job  *tjob.Job
}
type JobServer struct {
	proto.UnimplementedJobServer

	// $MAJ:$MIN device number for MNT namespace
	Mnt string

	// CPUPercent represents the quota of all cores.
	CPUPercent int

	// MemoryMB represents the quota of memory to in Megabytes.
	MemoryMB int

	// ReadBPS represents the max bytes read per second by proc
	ReadBPS int

	// WriteBPS represents the max bytes write per second by proc
	WriteBPS int

	jobs sync.Map
}

func (s *JobServer) Run(c context.Context, r *proto.RunRequest) (*proto.RunResponse, error) {
	user, err := s.userOf(c)
	if err != nil {
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	job := tjob.NewJob(r.Path, r.Args...)

	// TODO: add to client request
	job.Mnt = s.Mnt
	job.CPUPercent = s.CPUPercent
	job.MemoryMB = s.MemoryMB
	job.ReadBPS = s.ReadBPS
	job.WriteBPS = s.WriteBPS

	// TODO: replace with better uuid shortener
	id, _, _ := strings.Cut(job.Id, "-")
	rr := &proto.RunResponse{JobId: id}
	s.jobs.Store(id, &userJob{user: user, job: job})

	if err := job.Start(context.Background()); err != nil {
		return rr, fmt.Errorf("job start: %w", err)
	}
	return rr, nil
}

func (s *JobServer) userOf(c context.Context) (string, error) {
	peer, ok := peer.FromContext(c)
	if !ok {
		return "", ErrNoPeer
	}
	t, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", ErrNoTLSInfo
	}
	if len(t.State.PeerCertificates) == 0 {
		return "", ErrNoPeerCertificates
	}
	// assume first peer certificate
	return t.State.PeerCertificates[0].Subject.CommonName, nil
}

func (s *JobServer) jobOf(c context.Context, id string) (*userJob, error) {
	user, err := s.userOf(c)
	if err != nil {
		return nil, err
	}
	v, ok := s.jobs.Load(id)
	if !ok {
		return nil, ErrUnexpected
	}
	u, ok := v.(*userJob)
	if !ok {
		return nil, ErrUnexpected
	}
	if u.user != user {
		return nil, ErrUnauthorized
	}
	return u, nil
}

func (s *JobServer) Stop(c context.Context, r *proto.StopRequest) (*proto.StopResponse, error) {
	j, err := s.jobOf(c, r.JobId)
	if err != nil {
		return nil, err
	}

	if err := j.job.Stop(); err != nil {
		return nil, fmt.Errorf("job stop: %w", err)
	}
	return nil, nil
}

func (s *JobServer) Status(c context.Context, r *proto.StatusRequest) (*proto.StatusResponse, error) {
	j, err := s.jobOf(c, r.JobId)
	if err != nil {
		return nil, err
	}
	ss := j.job.Status()
	out := proto.Status{
		JobId:     r.JobId,
		Cmd:       ss.Cmd,
		StartedAt: timestamppb.New(ss.StartedAt),
		Ran:       durationpb.New(ss.Ran),
	}
	if ss.Stopped() {
		out.Exit = &ss.Exit
	}
	if ss.Error != nil {
		out.Error = ss.Error.Error()
	}
	return &proto.StatusResponse{Job: &out}, nil
}

func (s *JobServer) Logs(r *proto.LogsRequest, stream grpc.ServerStreamingServer[proto.LogsResponse]) error {
	c := stream.Context()
	j, err := s.jobOf(c, r.JobId)
	if err != nil {
		return err
	}

	logs, _ := j.job.Logs(c)
	defer func() { logs.Close() }()

	buffer := make([]byte, 1024)
	// poll logs to send back
	for {
		n, err := logs.Read(buffer)
		if n > 0 {
			out := &proto.LogsResponse{
				Out: buffer[:n],
			}
			if err = stream.Send(out); err != nil {
				return fmt.Errorf("stream send: %w", err)
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("job read: %w", err)
			}
			break
		}
	}
	return nil
}
