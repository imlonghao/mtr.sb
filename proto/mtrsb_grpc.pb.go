// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.12
// source: proto/mtrsb.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// MtrSbWorkerClient is the client API for MtrSbWorker service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type MtrSbWorkerClient interface {
	Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (MtrSbWorker_PingClient, error)
	Version(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*VersionResponse, error)
	Traceroute(ctx context.Context, in *TracerouteRequest, opts ...grpc.CallOption) (MtrSbWorker_TracerouteClient, error)
	Mtr(ctx context.Context, in *MtrRequest, opts ...grpc.CallOption) (MtrSbWorker_MtrClient, error)
}

type mtrSbWorkerClient struct {
	cc grpc.ClientConnInterface
}

func NewMtrSbWorkerClient(cc grpc.ClientConnInterface) MtrSbWorkerClient {
	return &mtrSbWorkerClient{cc}
}

func (c *mtrSbWorkerClient) Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (MtrSbWorker_PingClient, error) {
	stream, err := c.cc.NewStream(ctx, &MtrSbWorker_ServiceDesc.Streams[0], "/MtrSbWorker/Ping", opts...)
	if err != nil {
		return nil, err
	}
	x := &mtrSbWorkerPingClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type MtrSbWorker_PingClient interface {
	Recv() (*PingResponse, error)
	grpc.ClientStream
}

type mtrSbWorkerPingClient struct {
	grpc.ClientStream
}

func (x *mtrSbWorkerPingClient) Recv() (*PingResponse, error) {
	m := new(PingResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *mtrSbWorkerClient) Version(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*VersionResponse, error) {
	out := new(VersionResponse)
	err := c.cc.Invoke(ctx, "/MtrSbWorker/Version", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *mtrSbWorkerClient) Traceroute(ctx context.Context, in *TracerouteRequest, opts ...grpc.CallOption) (MtrSbWorker_TracerouteClient, error) {
	stream, err := c.cc.NewStream(ctx, &MtrSbWorker_ServiceDesc.Streams[1], "/MtrSbWorker/Traceroute", opts...)
	if err != nil {
		return nil, err
	}
	x := &mtrSbWorkerTracerouteClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type MtrSbWorker_TracerouteClient interface {
	Recv() (*TracerouteResponse, error)
	grpc.ClientStream
}

type mtrSbWorkerTracerouteClient struct {
	grpc.ClientStream
}

func (x *mtrSbWorkerTracerouteClient) Recv() (*TracerouteResponse, error) {
	m := new(TracerouteResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *mtrSbWorkerClient) Mtr(ctx context.Context, in *MtrRequest, opts ...grpc.CallOption) (MtrSbWorker_MtrClient, error) {
	stream, err := c.cc.NewStream(ctx, &MtrSbWorker_ServiceDesc.Streams[2], "/MtrSbWorker/Mtr", opts...)
	if err != nil {
		return nil, err
	}
	x := &mtrSbWorkerMtrClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type MtrSbWorker_MtrClient interface {
	Recv() (*MtrResponse, error)
	grpc.ClientStream
}

type mtrSbWorkerMtrClient struct {
	grpc.ClientStream
}

func (x *mtrSbWorkerMtrClient) Recv() (*MtrResponse, error) {
	m := new(MtrResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// MtrSbWorkerServer is the server API for MtrSbWorker service.
// All implementations must embed UnimplementedMtrSbWorkerServer
// for forward compatibility
type MtrSbWorkerServer interface {
	Ping(*PingRequest, MtrSbWorker_PingServer) error
	Version(context.Context, *VersionRequest) (*VersionResponse, error)
	Traceroute(*TracerouteRequest, MtrSbWorker_TracerouteServer) error
	Mtr(*MtrRequest, MtrSbWorker_MtrServer) error
	mustEmbedUnimplementedMtrSbWorkerServer()
}

// UnimplementedMtrSbWorkerServer must be embedded to have forward compatible implementations.
type UnimplementedMtrSbWorkerServer struct {
}

func (UnimplementedMtrSbWorkerServer) Ping(*PingRequest, MtrSbWorker_PingServer) error {
	return status.Errorf(codes.Unimplemented, "method Ping not implemented")
}
func (UnimplementedMtrSbWorkerServer) Version(context.Context, *VersionRequest) (*VersionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Version not implemented")
}
func (UnimplementedMtrSbWorkerServer) Traceroute(*TracerouteRequest, MtrSbWorker_TracerouteServer) error {
	return status.Errorf(codes.Unimplemented, "method Traceroute not implemented")
}
func (UnimplementedMtrSbWorkerServer) Mtr(*MtrRequest, MtrSbWorker_MtrServer) error {
	return status.Errorf(codes.Unimplemented, "method Mtr not implemented")
}
func (UnimplementedMtrSbWorkerServer) mustEmbedUnimplementedMtrSbWorkerServer() {}

// UnsafeMtrSbWorkerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to MtrSbWorkerServer will
// result in compilation errors.
type UnsafeMtrSbWorkerServer interface {
	mustEmbedUnimplementedMtrSbWorkerServer()
}

func RegisterMtrSbWorkerServer(s grpc.ServiceRegistrar, srv MtrSbWorkerServer) {
	s.RegisterService(&MtrSbWorker_ServiceDesc, srv)
}

func _MtrSbWorker_Ping_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(PingRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(MtrSbWorkerServer).Ping(m, &mtrSbWorkerPingServer{stream})
}

type MtrSbWorker_PingServer interface {
	Send(*PingResponse) error
	grpc.ServerStream
}

type mtrSbWorkerPingServer struct {
	grpc.ServerStream
}

func (x *mtrSbWorkerPingServer) Send(m *PingResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _MtrSbWorker_Version_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(VersionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MtrSbWorkerServer).Version(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/MtrSbWorker/Version",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MtrSbWorkerServer).Version(ctx, req.(*VersionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _MtrSbWorker_Traceroute_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(TracerouteRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(MtrSbWorkerServer).Traceroute(m, &mtrSbWorkerTracerouteServer{stream})
}

type MtrSbWorker_TracerouteServer interface {
	Send(*TracerouteResponse) error
	grpc.ServerStream
}

type mtrSbWorkerTracerouteServer struct {
	grpc.ServerStream
}

func (x *mtrSbWorkerTracerouteServer) Send(m *TracerouteResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _MtrSbWorker_Mtr_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(MtrRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(MtrSbWorkerServer).Mtr(m, &mtrSbWorkerMtrServer{stream})
}

type MtrSbWorker_MtrServer interface {
	Send(*MtrResponse) error
	grpc.ServerStream
}

type mtrSbWorkerMtrServer struct {
	grpc.ServerStream
}

func (x *mtrSbWorkerMtrServer) Send(m *MtrResponse) error {
	return x.ServerStream.SendMsg(m)
}

// MtrSbWorker_ServiceDesc is the grpc.ServiceDesc for MtrSbWorker service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var MtrSbWorker_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "MtrSbWorker",
	HandlerType: (*MtrSbWorkerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Version",
			Handler:    _MtrSbWorker_Version_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Ping",
			Handler:       _MtrSbWorker_Ping_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "Traceroute",
			Handler:       _MtrSbWorker_Traceroute_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "Mtr",
			Handler:       _MtrSbWorker_Mtr_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "proto/mtrsb.proto",
}
