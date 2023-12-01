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

// Code generated by protoc-gen-connect-go. DO NOT EDIT.
//
// Source: buf/alpha/registry/v1alpha1/doc.proto

package v1alpha1connect

import (
	connect "connectrpc.com/connect"
	context "context"
	errors "errors"
	v1alpha1 "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	http "net/http"
	strings "strings"
)

// This is a compile-time assertion to ensure that this generated file and the connect package are
// compatible. If you get a compiler error that this constant is not defined, this code was
// generated with a version of connect newer than the one compiled into your binary. You can fix the
// problem by either regenerating this code with an older version of connect or updating the connect
// version compiled into your binary.
const _ = connect.IsAtLeastVersion1_7_0

const (
	// DocServiceName is the fully-qualified name of the DocService service.
	DocServiceName = "buf.alpha.registry.v1alpha1.DocService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// DocServiceGetSourceDirectoryInfoProcedure is the fully-qualified name of the DocService's
	// GetSourceDirectoryInfo RPC.
	DocServiceGetSourceDirectoryInfoProcedure = "/buf.alpha.registry.v1alpha1.DocService/GetSourceDirectoryInfo"
	// DocServiceGetSourceFileProcedure is the fully-qualified name of the DocService's GetSourceFile
	// RPC.
	DocServiceGetSourceFileProcedure = "/buf.alpha.registry.v1alpha1.DocService/GetSourceFile"
	// DocServiceGetModulePackagesProcedure is the fully-qualified name of the DocService's
	// GetModulePackages RPC.
	DocServiceGetModulePackagesProcedure = "/buf.alpha.registry.v1alpha1.DocService/GetModulePackages"
	// DocServiceGetModuleDocumentationProcedure is the fully-qualified name of the DocService's
	// GetModuleDocumentation RPC.
	DocServiceGetModuleDocumentationProcedure = "/buf.alpha.registry.v1alpha1.DocService/GetModuleDocumentation"
	// DocServiceGetPackageDocumentationProcedure is the fully-qualified name of the DocService's
	// GetPackageDocumentation RPC.
	DocServiceGetPackageDocumentationProcedure = "/buf.alpha.registry.v1alpha1.DocService/GetPackageDocumentation"
)

// DocServiceClient is a client for the buf.alpha.registry.v1alpha1.DocService service.
type DocServiceClient interface {
	// GetSourceDirectoryInfo retrieves the directory and file structure for the
	// given owner, repository and reference.
	//
	// The purpose of this is to get a representation of the file tree for a given
	// module to enable exploring the module by navigating through its contents.
	GetSourceDirectoryInfo(context.Context, *connect.Request[v1alpha1.GetSourceDirectoryInfoRequest]) (*connect.Response[v1alpha1.GetSourceDirectoryInfoResponse], error)
	// GetSourceFile retrieves the source contents for the given owner, repository,
	// reference, and path.
	GetSourceFile(context.Context, *connect.Request[v1alpha1.GetSourceFileRequest]) (*connect.Response[v1alpha1.GetSourceFileResponse], error)
	// GetModulePackages retrieves the list of packages for the module based on the given
	// owner, repository, and reference.
	GetModulePackages(context.Context, *connect.Request[v1alpha1.GetModulePackagesRequest]) (*connect.Response[v1alpha1.GetModulePackagesResponse], error)
	// GetModuleDocumentation retrieves the documentations including buf.md and LICENSE files
	// for module based on the given owner, repository, and reference.
	GetModuleDocumentation(context.Context, *connect.Request[v1alpha1.GetModuleDocumentationRequest]) (*connect.Response[v1alpha1.GetModuleDocumentationResponse], error)
	// GetPackageDocumentation retrieves a a slice of documentation structures
	// for the given owner, repository, reference, and package name.
	GetPackageDocumentation(context.Context, *connect.Request[v1alpha1.GetPackageDocumentationRequest]) (*connect.Response[v1alpha1.GetPackageDocumentationResponse], error)
}

// NewDocServiceClient constructs a client for the buf.alpha.registry.v1alpha1.DocService service.
// By default, it uses the Connect protocol with the binary Protobuf Codec, asks for gzipped
// responses, and sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply the
// connect.WithGRPC() or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewDocServiceClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) DocServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &docServiceClient{
		getSourceDirectoryInfo: connect.NewClient[v1alpha1.GetSourceDirectoryInfoRequest, v1alpha1.GetSourceDirectoryInfoResponse](
			httpClient,
			baseURL+DocServiceGetSourceDirectoryInfoProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
		getSourceFile: connect.NewClient[v1alpha1.GetSourceFileRequest, v1alpha1.GetSourceFileResponse](
			httpClient,
			baseURL+DocServiceGetSourceFileProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
		getModulePackages: connect.NewClient[v1alpha1.GetModulePackagesRequest, v1alpha1.GetModulePackagesResponse](
			httpClient,
			baseURL+DocServiceGetModulePackagesProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
		getModuleDocumentation: connect.NewClient[v1alpha1.GetModuleDocumentationRequest, v1alpha1.GetModuleDocumentationResponse](
			httpClient,
			baseURL+DocServiceGetModuleDocumentationProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
		getPackageDocumentation: connect.NewClient[v1alpha1.GetPackageDocumentationRequest, v1alpha1.GetPackageDocumentationResponse](
			httpClient,
			baseURL+DocServiceGetPackageDocumentationProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
	}
}

// docServiceClient implements DocServiceClient.
type docServiceClient struct {
	getSourceDirectoryInfo  *connect.Client[v1alpha1.GetSourceDirectoryInfoRequest, v1alpha1.GetSourceDirectoryInfoResponse]
	getSourceFile           *connect.Client[v1alpha1.GetSourceFileRequest, v1alpha1.GetSourceFileResponse]
	getModulePackages       *connect.Client[v1alpha1.GetModulePackagesRequest, v1alpha1.GetModulePackagesResponse]
	getModuleDocumentation  *connect.Client[v1alpha1.GetModuleDocumentationRequest, v1alpha1.GetModuleDocumentationResponse]
	getPackageDocumentation *connect.Client[v1alpha1.GetPackageDocumentationRequest, v1alpha1.GetPackageDocumentationResponse]
}

// GetSourceDirectoryInfo calls buf.alpha.registry.v1alpha1.DocService.GetSourceDirectoryInfo.
func (c *docServiceClient) GetSourceDirectoryInfo(ctx context.Context, req *connect.Request[v1alpha1.GetSourceDirectoryInfoRequest]) (*connect.Response[v1alpha1.GetSourceDirectoryInfoResponse], error) {
	return c.getSourceDirectoryInfo.CallUnary(ctx, req)
}

// GetSourceFile calls buf.alpha.registry.v1alpha1.DocService.GetSourceFile.
func (c *docServiceClient) GetSourceFile(ctx context.Context, req *connect.Request[v1alpha1.GetSourceFileRequest]) (*connect.Response[v1alpha1.GetSourceFileResponse], error) {
	return c.getSourceFile.CallUnary(ctx, req)
}

// GetModulePackages calls buf.alpha.registry.v1alpha1.DocService.GetModulePackages.
func (c *docServiceClient) GetModulePackages(ctx context.Context, req *connect.Request[v1alpha1.GetModulePackagesRequest]) (*connect.Response[v1alpha1.GetModulePackagesResponse], error) {
	return c.getModulePackages.CallUnary(ctx, req)
}

// GetModuleDocumentation calls buf.alpha.registry.v1alpha1.DocService.GetModuleDocumentation.
func (c *docServiceClient) GetModuleDocumentation(ctx context.Context, req *connect.Request[v1alpha1.GetModuleDocumentationRequest]) (*connect.Response[v1alpha1.GetModuleDocumentationResponse], error) {
	return c.getModuleDocumentation.CallUnary(ctx, req)
}

// GetPackageDocumentation calls buf.alpha.registry.v1alpha1.DocService.GetPackageDocumentation.
func (c *docServiceClient) GetPackageDocumentation(ctx context.Context, req *connect.Request[v1alpha1.GetPackageDocumentationRequest]) (*connect.Response[v1alpha1.GetPackageDocumentationResponse], error) {
	return c.getPackageDocumentation.CallUnary(ctx, req)
}

// DocServiceHandler is an implementation of the buf.alpha.registry.v1alpha1.DocService service.
type DocServiceHandler interface {
	// GetSourceDirectoryInfo retrieves the directory and file structure for the
	// given owner, repository and reference.
	//
	// The purpose of this is to get a representation of the file tree for a given
	// module to enable exploring the module by navigating through its contents.
	GetSourceDirectoryInfo(context.Context, *connect.Request[v1alpha1.GetSourceDirectoryInfoRequest]) (*connect.Response[v1alpha1.GetSourceDirectoryInfoResponse], error)
	// GetSourceFile retrieves the source contents for the given owner, repository,
	// reference, and path.
	GetSourceFile(context.Context, *connect.Request[v1alpha1.GetSourceFileRequest]) (*connect.Response[v1alpha1.GetSourceFileResponse], error)
	// GetModulePackages retrieves the list of packages for the module based on the given
	// owner, repository, and reference.
	GetModulePackages(context.Context, *connect.Request[v1alpha1.GetModulePackagesRequest]) (*connect.Response[v1alpha1.GetModulePackagesResponse], error)
	// GetModuleDocumentation retrieves the documentations including buf.md and LICENSE files
	// for module based on the given owner, repository, and reference.
	GetModuleDocumentation(context.Context, *connect.Request[v1alpha1.GetModuleDocumentationRequest]) (*connect.Response[v1alpha1.GetModuleDocumentationResponse], error)
	// GetPackageDocumentation retrieves a a slice of documentation structures
	// for the given owner, repository, reference, and package name.
	GetPackageDocumentation(context.Context, *connect.Request[v1alpha1.GetPackageDocumentationRequest]) (*connect.Response[v1alpha1.GetPackageDocumentationResponse], error)
}

// NewDocServiceHandler builds an HTTP handler from the service implementation. It returns the path
// on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewDocServiceHandler(svc DocServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	docServiceGetSourceDirectoryInfoHandler := connect.NewUnaryHandler(
		DocServiceGetSourceDirectoryInfoProcedure,
		svc.GetSourceDirectoryInfo,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	docServiceGetSourceFileHandler := connect.NewUnaryHandler(
		DocServiceGetSourceFileProcedure,
		svc.GetSourceFile,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	docServiceGetModulePackagesHandler := connect.NewUnaryHandler(
		DocServiceGetModulePackagesProcedure,
		svc.GetModulePackages,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	docServiceGetModuleDocumentationHandler := connect.NewUnaryHandler(
		DocServiceGetModuleDocumentationProcedure,
		svc.GetModuleDocumentation,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	docServiceGetPackageDocumentationHandler := connect.NewUnaryHandler(
		DocServiceGetPackageDocumentationProcedure,
		svc.GetPackageDocumentation,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	return "/buf.alpha.registry.v1alpha1.DocService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DocServiceGetSourceDirectoryInfoProcedure:
			docServiceGetSourceDirectoryInfoHandler.ServeHTTP(w, r)
		case DocServiceGetSourceFileProcedure:
			docServiceGetSourceFileHandler.ServeHTTP(w, r)
		case DocServiceGetModulePackagesProcedure:
			docServiceGetModulePackagesHandler.ServeHTTP(w, r)
		case DocServiceGetModuleDocumentationProcedure:
			docServiceGetModuleDocumentationHandler.ServeHTTP(w, r)
		case DocServiceGetPackageDocumentationProcedure:
			docServiceGetPackageDocumentationHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedDocServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedDocServiceHandler struct{}

func (UnimplementedDocServiceHandler) GetSourceDirectoryInfo(context.Context, *connect.Request[v1alpha1.GetSourceDirectoryInfoRequest]) (*connect.Response[v1alpha1.GetSourceDirectoryInfoResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.DocService.GetSourceDirectoryInfo is not implemented"))
}

func (UnimplementedDocServiceHandler) GetSourceFile(context.Context, *connect.Request[v1alpha1.GetSourceFileRequest]) (*connect.Response[v1alpha1.GetSourceFileResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.DocService.GetSourceFile is not implemented"))
}

func (UnimplementedDocServiceHandler) GetModulePackages(context.Context, *connect.Request[v1alpha1.GetModulePackagesRequest]) (*connect.Response[v1alpha1.GetModulePackagesResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.DocService.GetModulePackages is not implemented"))
}

func (UnimplementedDocServiceHandler) GetModuleDocumentation(context.Context, *connect.Request[v1alpha1.GetModuleDocumentationRequest]) (*connect.Response[v1alpha1.GetModuleDocumentationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.DocService.GetModuleDocumentation is not implemented"))
}

func (UnimplementedDocServiceHandler) GetPackageDocumentation(context.Context, *connect.Request[v1alpha1.GetPackageDocumentationRequest]) (*connect.Response[v1alpha1.GetPackageDocumentationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.DocService.GetPackageDocumentation is not implemented"))
}
