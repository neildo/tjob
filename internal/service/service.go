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
	ErrNotFound           = errors.New("not found")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrUnexpected         = errors.New("unexpected")
	ErrNoPeer             = errors.New("no peer")
	ErrNoTLSInfo          = errors.New("no TLS info")
	ErrNoPeerCertificates = errors.New("no peer certificates")
)

type userJob struct {
	user string
	job  *tjob.Job
}

// JobServer is an implementation of the proto.JobServer interface.
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

// Run starts a new job for originating user only
func (s *JobServer) Run(c context.Context, req *proto.RunRequest) (*proto.RunResponse, error) {
	user, err := s.userOf(c)
	if err != nil {
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	job := tjob.NewJob(req.GetPath(), req.GetArgs()...)

	// TODO: add to client request
	job.Mnt = s.Mnt
	job.CPUPercent = s.CPUPercent
	job.MemoryMB = s.MemoryMB
	job.ReadBPS = s.ReadBPS
	job.WriteBPS = s.WriteBPS

	// TODO: replace with better uuid shortener
	id, _, _ := strings.Cut(job.Id, "-")
	resp := &proto.RunResponse{JobId: id}
	s.jobs.Store(id, &userJob{user: user, job: job})

	if err := job.Start(context.Background()); err != nil { //nolint:contextcheck
		return resp, fmt.Errorf("job start: %w", err)
	}
	return resp, nil
}

// Stop stops job for originating user only
func (s *JobServer) Stop(c context.Context, req *proto.StopRequest) (*proto.StopResponse, error) {
	j, err := s.jobOf(c, req.GetJobId())
	if err != nil {
		return nil, err
	}

	if err := j.job.Stop(); err != nil {
		return nil, fmt.Errorf("job stop: %w", err)
	}
	return &proto.StopResponse{}, nil
}

// Status returns status of job for originating user only
func (s *JobServer) Status(c context.Context, req *proto.StatusRequest) (*proto.StatusResponse, error) {
	j, err := s.jobOf(c, req.GetJobId())
	if err != nil {
		return nil, err
	}
	status := j.job.Status()
	out := proto.Status{
		JobId:     req.GetJobId(),
		Cmd:       status.Cmd,
		StartedAt: timestamppb.New(status.StartedAt),
		Ran:       durationpb.New(status.Ran),
	}
	if status.Stopped() {
		out.Exit = &status.Exit
	}
	if status.Error != nil {
		out.Error = status.Error.Error()
	}
	return &proto.StatusResponse{Job: &out}, nil
}

// Logs streams logs for running job or history of logs completed job for originating user only
func (s *JobServer) Logs(req *proto.LogsRequest, stream grpc.ServerStreamingServer[proto.LogsResponse]) error {
	ctx := stream.Context()
	j, err := s.jobOf(ctx, req.GetJobId())
	if err != nil {
		return err
	}

	logs, _ := j.job.Logs(ctx)
	defer func() { logs.Close() }()

	buffer := make([]byte, 1024)
	// poll logs to send back
	for {
		n, err := logs.Read(buffer)
		if n > 0 {
			out := &proto.LogsResponse{
				Out: make([]byte, n),
			}
			copy(out.Out, buffer[:n])
			if err = stream.Send(out); err != nil {
				return fmt.Errorf("stream send: %w", err)
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("log read: %w", err)
			}
			break
		}
	}
	return nil
}

func (s *JobServer) userOf(c context.Context) (string, error) {
	peer, ok := peer.FromContext(c)
	if !ok {
		return "", ErrNoPeer
	}
	info, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", ErrNoTLSInfo
	}
	if len(info.State.PeerCertificates) == 0 {
		return "", ErrNoPeerCertificates
	}
	// assume first peer certificate
	return info.State.PeerCertificates[0].Subject.CommonName, nil
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
	job, ok := v.(*userJob)
	if !ok {
		return nil, ErrUnexpected
	}
	if job.user != user {
		return nil, ErrUnauthorized
	}
	return job, nil
}
