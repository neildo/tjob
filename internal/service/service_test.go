package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"github.com/neildo/tjob"
	"github.com/neildo/tjob/internal/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// asUser returns a context carrying a verified client certificate for user.
func asUser(user string) context.Context {
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: user}}
	return peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}},
	})
}

func TestJobOfHidesOtherUsersJobs(t *testing.T) {
	t.Parallel()

	s := &JobServer{}
	s.jobs.Store("alice-job", &userJob{user: "alice", job: tjob.NewJob("/bin/true")})

	for _, id := range []string{"alice-job", "missing"} {
		_, err := s.Status(asUser("bob"), &proto.StatusRequest{JobId: id})
		if got := status.Code(err); got != codes.NotFound {
			t.Errorf("status(%s) as bob = %v, want NotFound", id, got)
		}
	}
}

func TestNoClientCertIsUnauthenticated(t *testing.T) {
	t.Parallel()

	s := &JobServer{}
	_, err := s.Status(context.Background(), &proto.StatusRequest{JobId: "any"})
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Errorf("status without peer = %v, want Unauthenticated", got)
	}
}

func TestStopUnstartedJob(t *testing.T) {
	t.Parallel()

	s := &JobServer{}
	s.jobs.Store("alice-job", &userJob{user: "alice", job: tjob.NewJob("/bin/true")})

	_, err := s.Stop(asUser("alice"), &proto.StopRequest{JobId: "alice-job"})
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("stop = %v, want FailedPrecondition", err)
	}
}
