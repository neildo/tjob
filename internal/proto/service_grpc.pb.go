// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v3.12.4
// source: internal/proto/service.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Job_Run_FullMethodName    = "/Job/Run"
	Job_Stop_FullMethodName   = "/Job/Stop"
	Job_Status_FullMethodName = "/Job/Status"
	Job_Logs_FullMethodName   = "/Job/Logs"
)

// JobClient is the client API for Job service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type JobClient interface {
	Run(ctx context.Context, in *RunRequest, opts ...grpc.CallOption) (*RunResponse, error)
	Stop(ctx context.Context, in *StopRequest, opts ...grpc.CallOption) (*StopResponse, error)
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
	Logs(ctx context.Context, in *LogsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[LogsResponse], error)
}

type jobClient struct {
	cc grpc.ClientConnInterface
}

func NewJobClient(cc grpc.ClientConnInterface) JobClient {
	return &jobClient{cc}
}

func (c *jobClient) Run(ctx context.Context, in *RunRequest, opts ...grpc.CallOption) (*RunResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(RunResponse)
	err := c.cc.Invoke(ctx, Job_Run_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jobClient) Stop(ctx context.Context, in *StopRequest, opts ...grpc.CallOption) (*StopResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(StopResponse)
	err := c.cc.Invoke(ctx, Job_Stop_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jobClient) Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, Job_Status_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jobClient) Logs(ctx context.Context, in *LogsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[LogsResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Job_ServiceDesc.Streams[0], Job_Logs_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[LogsRequest, LogsResponse]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Job_LogsClient = grpc.ServerStreamingClient[LogsResponse]

// JobServer is the server API for Job service.
// All implementations must embed UnimplementedJobServer
// for forward compatibility.
type JobServer interface {
	Run(context.Context, *RunRequest) (*RunResponse, error)
	Stop(context.Context, *StopRequest) (*StopResponse, error)
	Status(context.Context, *StatusRequest) (*StatusResponse, error)
	Logs(*LogsRequest, grpc.ServerStreamingServer[LogsResponse]) error
	mustEmbedUnimplementedJobServer()
}

// UnimplementedJobServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedJobServer struct{}

func (UnimplementedJobServer) Run(context.Context, *RunRequest) (*RunResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Run not implemented")
}
func (UnimplementedJobServer) Stop(context.Context, *StopRequest) (*StopResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Stop not implemented")
}
func (UnimplementedJobServer) Status(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedJobServer) Logs(*LogsRequest, grpc.ServerStreamingServer[LogsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method Logs not implemented")
}
func (UnimplementedJobServer) mustEmbedUnimplementedJobServer() {}
func (UnimplementedJobServer) testEmbeddedByValue()             {}

// UnsafeJobServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to JobServer will
// result in compilation errors.
type UnsafeJobServer interface {
	mustEmbedUnimplementedJobServer()
}

func RegisterJobServer(s grpc.ServiceRegistrar, srv JobServer) {
	// If the following call pancis, it indicates UnimplementedJobServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Job_ServiceDesc, srv)
}

func _Job_Run_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RunRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JobServer).Run(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Job_Run_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JobServer).Run(ctx, req.(*RunRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Job_Stop_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JobServer).Stop(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Job_Stop_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JobServer).Stop(ctx, req.(*StopRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Job_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JobServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Job_Status_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JobServer).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Job_Logs_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(LogsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(JobServer).Logs(m, &grpc.GenericServerStream[LogsRequest, LogsResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Job_LogsServer = grpc.ServerStreamingServer[LogsResponse]

// Job_ServiceDesc is the grpc.ServiceDesc for Job service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Job_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "Job",
	HandlerType: (*JobServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Run",
			Handler:    _Job_Run_Handler,
		},
		{
			MethodName: "Stop",
			Handler:    _Job_Stop_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _Job_Status_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Logs",
			Handler:       _Job_Logs_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "internal/proto/service.proto",
}
