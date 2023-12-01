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
// Source: buf/alpha/registry/v1alpha1/token.proto

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
	// TokenServiceName is the fully-qualified name of the TokenService service.
	TokenServiceName = "buf.alpha.registry.v1alpha1.TokenService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// TokenServiceCreateTokenProcedure is the fully-qualified name of the TokenService's CreateToken
	// RPC.
	TokenServiceCreateTokenProcedure = "/buf.alpha.registry.v1alpha1.TokenService/CreateToken"
	// TokenServiceGetTokenProcedure is the fully-qualified name of the TokenService's GetToken RPC.
	TokenServiceGetTokenProcedure = "/buf.alpha.registry.v1alpha1.TokenService/GetToken"
	// TokenServiceListTokensProcedure is the fully-qualified name of the TokenService's ListTokens RPC.
	TokenServiceListTokensProcedure = "/buf.alpha.registry.v1alpha1.TokenService/ListTokens"
	// TokenServiceDeleteTokenProcedure is the fully-qualified name of the TokenService's DeleteToken
	// RPC.
	TokenServiceDeleteTokenProcedure = "/buf.alpha.registry.v1alpha1.TokenService/DeleteToken"
)

// TokenServiceClient is a client for the buf.alpha.registry.v1alpha1.TokenService service.
type TokenServiceClient interface {
	// CreateToken creates a new token suitable for machine-to-machine authentication.
	CreateToken(context.Context, *connect.Request[v1alpha1.CreateTokenRequest]) (*connect.Response[v1alpha1.CreateTokenResponse], error)
	// GetToken gets the specific token for the user
	//
	// This method requires authentication.
	GetToken(context.Context, *connect.Request[v1alpha1.GetTokenRequest]) (*connect.Response[v1alpha1.GetTokenResponse], error)
	// ListTokens lists the users active tokens
	//
	// This method requires authentication.
	ListTokens(context.Context, *connect.Request[v1alpha1.ListTokensRequest]) (*connect.Response[v1alpha1.ListTokensResponse], error)
	// DeleteToken deletes an existing token.
	//
	// This method requires authentication.
	DeleteToken(context.Context, *connect.Request[v1alpha1.DeleteTokenRequest]) (*connect.Response[v1alpha1.DeleteTokenResponse], error)
}

// NewTokenServiceClient constructs a client for the buf.alpha.registry.v1alpha1.TokenService
// service. By default, it uses the Connect protocol with the binary Protobuf Codec, asks for
// gzipped responses, and sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply
// the connect.WithGRPC() or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewTokenServiceClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) TokenServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &tokenServiceClient{
		createToken: connect.NewClient[v1alpha1.CreateTokenRequest, v1alpha1.CreateTokenResponse](
			httpClient,
			baseURL+TokenServiceCreateTokenProcedure,
			opts...,
		),
		getToken: connect.NewClient[v1alpha1.GetTokenRequest, v1alpha1.GetTokenResponse](
			httpClient,
			baseURL+TokenServiceGetTokenProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
		listTokens: connect.NewClient[v1alpha1.ListTokensRequest, v1alpha1.ListTokensResponse](
			httpClient,
			baseURL+TokenServiceListTokensProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
		deleteToken: connect.NewClient[v1alpha1.DeleteTokenRequest, v1alpha1.DeleteTokenResponse](
			httpClient,
			baseURL+TokenServiceDeleteTokenProcedure,
			connect.WithIdempotency(connect.IdempotencyIdempotent),
			connect.WithClientOptions(opts...),
		),
	}
}

// tokenServiceClient implements TokenServiceClient.
type tokenServiceClient struct {
	createToken *connect.Client[v1alpha1.CreateTokenRequest, v1alpha1.CreateTokenResponse]
	getToken    *connect.Client[v1alpha1.GetTokenRequest, v1alpha1.GetTokenResponse]
	listTokens  *connect.Client[v1alpha1.ListTokensRequest, v1alpha1.ListTokensResponse]
	deleteToken *connect.Client[v1alpha1.DeleteTokenRequest, v1alpha1.DeleteTokenResponse]
}

// CreateToken calls buf.alpha.registry.v1alpha1.TokenService.CreateToken.
func (c *tokenServiceClient) CreateToken(ctx context.Context, req *connect.Request[v1alpha1.CreateTokenRequest]) (*connect.Response[v1alpha1.CreateTokenResponse], error) {
	return c.createToken.CallUnary(ctx, req)
}

// GetToken calls buf.alpha.registry.v1alpha1.TokenService.GetToken.
func (c *tokenServiceClient) GetToken(ctx context.Context, req *connect.Request[v1alpha1.GetTokenRequest]) (*connect.Response[v1alpha1.GetTokenResponse], error) {
	return c.getToken.CallUnary(ctx, req)
}

// ListTokens calls buf.alpha.registry.v1alpha1.TokenService.ListTokens.
func (c *tokenServiceClient) ListTokens(ctx context.Context, req *connect.Request[v1alpha1.ListTokensRequest]) (*connect.Response[v1alpha1.ListTokensResponse], error) {
	return c.listTokens.CallUnary(ctx, req)
}

// DeleteToken calls buf.alpha.registry.v1alpha1.TokenService.DeleteToken.
func (c *tokenServiceClient) DeleteToken(ctx context.Context, req *connect.Request[v1alpha1.DeleteTokenRequest]) (*connect.Response[v1alpha1.DeleteTokenResponse], error) {
	return c.deleteToken.CallUnary(ctx, req)
}

// TokenServiceHandler is an implementation of the buf.alpha.registry.v1alpha1.TokenService service.
type TokenServiceHandler interface {
	// CreateToken creates a new token suitable for machine-to-machine authentication.
	CreateToken(context.Context, *connect.Request[v1alpha1.CreateTokenRequest]) (*connect.Response[v1alpha1.CreateTokenResponse], error)
	// GetToken gets the specific token for the user
	//
	// This method requires authentication.
	GetToken(context.Context, *connect.Request[v1alpha1.GetTokenRequest]) (*connect.Response[v1alpha1.GetTokenResponse], error)
	// ListTokens lists the users active tokens
	//
	// This method requires authentication.
	ListTokens(context.Context, *connect.Request[v1alpha1.ListTokensRequest]) (*connect.Response[v1alpha1.ListTokensResponse], error)
	// DeleteToken deletes an existing token.
	//
	// This method requires authentication.
	DeleteToken(context.Context, *connect.Request[v1alpha1.DeleteTokenRequest]) (*connect.Response[v1alpha1.DeleteTokenResponse], error)
}

// NewTokenServiceHandler builds an HTTP handler from the service implementation. It returns the
// path on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewTokenServiceHandler(svc TokenServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	tokenServiceCreateTokenHandler := connect.NewUnaryHandler(
		TokenServiceCreateTokenProcedure,
		svc.CreateToken,
		opts...,
	)
	tokenServiceGetTokenHandler := connect.NewUnaryHandler(
		TokenServiceGetTokenProcedure,
		svc.GetToken,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	tokenServiceListTokensHandler := connect.NewUnaryHandler(
		TokenServiceListTokensProcedure,
		svc.ListTokens,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	tokenServiceDeleteTokenHandler := connect.NewUnaryHandler(
		TokenServiceDeleteTokenProcedure,
		svc.DeleteToken,
		connect.WithIdempotency(connect.IdempotencyIdempotent),
		connect.WithHandlerOptions(opts...),
	)
	return "/buf.alpha.registry.v1alpha1.TokenService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case TokenServiceCreateTokenProcedure:
			tokenServiceCreateTokenHandler.ServeHTTP(w, r)
		case TokenServiceGetTokenProcedure:
			tokenServiceGetTokenHandler.ServeHTTP(w, r)
		case TokenServiceListTokensProcedure:
			tokenServiceListTokensHandler.ServeHTTP(w, r)
		case TokenServiceDeleteTokenProcedure:
			tokenServiceDeleteTokenHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedTokenServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedTokenServiceHandler struct{}

func (UnimplementedTokenServiceHandler) CreateToken(context.Context, *connect.Request[v1alpha1.CreateTokenRequest]) (*connect.Response[v1alpha1.CreateTokenResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.TokenService.CreateToken is not implemented"))
}

func (UnimplementedTokenServiceHandler) GetToken(context.Context, *connect.Request[v1alpha1.GetTokenRequest]) (*connect.Response[v1alpha1.GetTokenResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.TokenService.GetToken is not implemented"))
}

func (UnimplementedTokenServiceHandler) ListTokens(context.Context, *connect.Request[v1alpha1.ListTokensRequest]) (*connect.Response[v1alpha1.ListTokensResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.TokenService.ListTokens is not implemented"))
}

func (UnimplementedTokenServiceHandler) DeleteToken(context.Context, *connect.Request[v1alpha1.DeleteTokenRequest]) (*connect.Response[v1alpha1.DeleteTokenResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.TokenService.DeleteToken is not implemented"))
}
