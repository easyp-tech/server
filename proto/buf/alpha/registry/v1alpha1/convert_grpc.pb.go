// Copyright 2020-2023 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             (unknown)
// source: buf/alpha/registry/v1alpha1/convert.proto

package registryv1alpha1

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

const (
	ConvertService_Convert_FullMethodName = "/buf.alpha.registry.v1alpha1.ConvertService/Convert"
)

// ConvertServiceClient is the client API for ConvertService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ConvertServiceClient interface {
	// Convert converts a serialized message according to
	// the provided type name using an image.
	Convert(ctx context.Context, in *ConvertRequest, opts ...grpc.CallOption) (*ConvertResponse, error)
}

type convertServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewConvertServiceClient(cc grpc.ClientConnInterface) ConvertServiceClient {
	return &convertServiceClient{cc}
}

func (c *convertServiceClient) Convert(ctx context.Context, in *ConvertRequest, opts ...grpc.CallOption) (*ConvertResponse, error) {
	out := new(ConvertResponse)
	err := c.cc.Invoke(ctx, ConvertService_Convert_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ConvertServiceServer is the server API for ConvertService service.
// All implementations should embed UnimplementedConvertServiceServer
// for forward compatibility
type ConvertServiceServer interface {
	// Convert converts a serialized message according to
	// the provided type name using an image.
	Convert(context.Context, *ConvertRequest) (*ConvertResponse, error)
}

// UnimplementedConvertServiceServer should be embedded to have forward compatible implementations.
type UnimplementedConvertServiceServer struct {
}

func (UnimplementedConvertServiceServer) Convert(context.Context, *ConvertRequest) (*ConvertResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Convert not implemented")
}

// UnsafeConvertServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ConvertServiceServer will
// result in compilation errors.
type UnsafeConvertServiceServer interface {
	mustEmbedUnimplementedConvertServiceServer()
}

func RegisterConvertServiceServer(s grpc.ServiceRegistrar, srv ConvertServiceServer) {
	s.RegisterService(&ConvertService_ServiceDesc, srv)
}

func _ConvertService_Convert_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ConvertRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConvertServiceServer).Convert(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ConvertService_Convert_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConvertServiceServer).Convert(ctx, req.(*ConvertRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// ConvertService_ServiceDesc is the grpc.ServiceDesc for ConvertService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ConvertService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "buf.alpha.registry.v1alpha1.ConvertService",
	HandlerType: (*ConvertServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Convert",
			Handler:    _ConvertService_Convert_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "buf/alpha/registry/v1alpha1/convert.proto",
}