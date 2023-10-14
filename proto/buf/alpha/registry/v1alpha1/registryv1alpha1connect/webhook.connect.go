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
// Source: buf/alpha/registry/v1alpha1/webhook.proto

package registryv1alpha1connect

import (
	connect "connectrpc.com/connect"
	context "context"
	errors "errors"
	v1alpha1 "github.com/easyp-tech/server/proto/buf/alpha/registry/v1alpha1"
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
	// WebhookServiceName is the fully-qualified name of the WebhookService service.
	WebhookServiceName = "buf.alpha.registry.v1alpha1.WebhookService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// WebhookServiceCreateWebhookProcedure is the fully-qualified name of the WebhookService's
	// CreateWebhook RPC.
	WebhookServiceCreateWebhookProcedure = "/buf.alpha.registry.v1alpha1.WebhookService/CreateWebhook"
	// WebhookServiceDeleteWebhookProcedure is the fully-qualified name of the WebhookService's
	// DeleteWebhook RPC.
	WebhookServiceDeleteWebhookProcedure = "/buf.alpha.registry.v1alpha1.WebhookService/DeleteWebhook"
	// WebhookServiceListWebhooksProcedure is the fully-qualified name of the WebhookService's
	// ListWebhooks RPC.
	WebhookServiceListWebhooksProcedure = "/buf.alpha.registry.v1alpha1.WebhookService/ListWebhooks"
)

// WebhookServiceClient is a client for the buf.alpha.registry.v1alpha1.WebhookService service.
type WebhookServiceClient interface {
	// Create a webhook, subscribes to a given repository event for a callback URL
	// invocation.
	CreateWebhook(context.Context, *connect.Request[v1alpha1.CreateWebhookRequest]) (*connect.Response[v1alpha1.CreateWebhookResponse], error)
	// Delete a webhook removes the event subscription.
	DeleteWebhook(context.Context, *connect.Request[v1alpha1.DeleteWebhookRequest]) (*connect.Response[v1alpha1.DeleteWebhookResponse], error)
	// Lists the webhooks subscriptions for a given repository.
	ListWebhooks(context.Context, *connect.Request[v1alpha1.ListWebhooksRequest]) (*connect.Response[v1alpha1.ListWebhooksResponse], error)
}

// NewWebhookServiceClient constructs a client for the buf.alpha.registry.v1alpha1.WebhookService
// service. By default, it uses the Connect protocol with the binary Protobuf Codec, asks for
// gzipped responses, and sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply
// the connect.WithGRPC() or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewWebhookServiceClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) WebhookServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &webhookServiceClient{
		createWebhook: connect.NewClient[v1alpha1.CreateWebhookRequest, v1alpha1.CreateWebhookResponse](
			httpClient,
			baseURL+WebhookServiceCreateWebhookProcedure,
			connect.WithIdempotency(connect.IdempotencyIdempotent),
			connect.WithClientOptions(opts...),
		),
		deleteWebhook: connect.NewClient[v1alpha1.DeleteWebhookRequest, v1alpha1.DeleteWebhookResponse](
			httpClient,
			baseURL+WebhookServiceDeleteWebhookProcedure,
			connect.WithIdempotency(connect.IdempotencyIdempotent),
			connect.WithClientOptions(opts...),
		),
		listWebhooks: connect.NewClient[v1alpha1.ListWebhooksRequest, v1alpha1.ListWebhooksResponse](
			httpClient,
			baseURL+WebhookServiceListWebhooksProcedure,
			connect.WithIdempotency(connect.IdempotencyNoSideEffects),
			connect.WithClientOptions(opts...),
		),
	}
}

// webhookServiceClient implements WebhookServiceClient.
type webhookServiceClient struct {
	createWebhook *connect.Client[v1alpha1.CreateWebhookRequest, v1alpha1.CreateWebhookResponse]
	deleteWebhook *connect.Client[v1alpha1.DeleteWebhookRequest, v1alpha1.DeleteWebhookResponse]
	listWebhooks  *connect.Client[v1alpha1.ListWebhooksRequest, v1alpha1.ListWebhooksResponse]
}

// CreateWebhook calls buf.alpha.registry.v1alpha1.WebhookService.CreateWebhook.
func (c *webhookServiceClient) CreateWebhook(ctx context.Context, req *connect.Request[v1alpha1.CreateWebhookRequest]) (*connect.Response[v1alpha1.CreateWebhookResponse], error) {
	return c.createWebhook.CallUnary(ctx, req)
}

// DeleteWebhook calls buf.alpha.registry.v1alpha1.WebhookService.DeleteWebhook.
func (c *webhookServiceClient) DeleteWebhook(ctx context.Context, req *connect.Request[v1alpha1.DeleteWebhookRequest]) (*connect.Response[v1alpha1.DeleteWebhookResponse], error) {
	return c.deleteWebhook.CallUnary(ctx, req)
}

// ListWebhooks calls buf.alpha.registry.v1alpha1.WebhookService.ListWebhooks.
func (c *webhookServiceClient) ListWebhooks(ctx context.Context, req *connect.Request[v1alpha1.ListWebhooksRequest]) (*connect.Response[v1alpha1.ListWebhooksResponse], error) {
	return c.listWebhooks.CallUnary(ctx, req)
}

// WebhookServiceHandler is an implementation of the buf.alpha.registry.v1alpha1.WebhookService
// service.
type WebhookServiceHandler interface {
	// Create a webhook, subscribes to a given repository event for a callback URL
	// invocation.
	CreateWebhook(context.Context, *connect.Request[v1alpha1.CreateWebhookRequest]) (*connect.Response[v1alpha1.CreateWebhookResponse], error)
	// Delete a webhook removes the event subscription.
	DeleteWebhook(context.Context, *connect.Request[v1alpha1.DeleteWebhookRequest]) (*connect.Response[v1alpha1.DeleteWebhookResponse], error)
	// Lists the webhooks subscriptions for a given repository.
	ListWebhooks(context.Context, *connect.Request[v1alpha1.ListWebhooksRequest]) (*connect.Response[v1alpha1.ListWebhooksResponse], error)
}

// NewWebhookServiceHandler builds an HTTP handler from the service implementation. It returns the
// path on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewWebhookServiceHandler(svc WebhookServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	webhookServiceCreateWebhookHandler := connect.NewUnaryHandler(
		WebhookServiceCreateWebhookProcedure,
		svc.CreateWebhook,
		connect.WithIdempotency(connect.IdempotencyIdempotent),
		connect.WithHandlerOptions(opts...),
	)
	webhookServiceDeleteWebhookHandler := connect.NewUnaryHandler(
		WebhookServiceDeleteWebhookProcedure,
		svc.DeleteWebhook,
		connect.WithIdempotency(connect.IdempotencyIdempotent),
		connect.WithHandlerOptions(opts...),
	)
	webhookServiceListWebhooksHandler := connect.NewUnaryHandler(
		WebhookServiceListWebhooksProcedure,
		svc.ListWebhooks,
		connect.WithIdempotency(connect.IdempotencyNoSideEffects),
		connect.WithHandlerOptions(opts...),
	)
	return "/buf.alpha.registry.v1alpha1.WebhookService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case WebhookServiceCreateWebhookProcedure:
			webhookServiceCreateWebhookHandler.ServeHTTP(w, r)
		case WebhookServiceDeleteWebhookProcedure:
			webhookServiceDeleteWebhookHandler.ServeHTTP(w, r)
		case WebhookServiceListWebhooksProcedure:
			webhookServiceListWebhooksHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedWebhookServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedWebhookServiceHandler struct{}

func (UnimplementedWebhookServiceHandler) CreateWebhook(context.Context, *connect.Request[v1alpha1.CreateWebhookRequest]) (*connect.Response[v1alpha1.CreateWebhookResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.WebhookService.CreateWebhook is not implemented"))
}

func (UnimplementedWebhookServiceHandler) DeleteWebhook(context.Context, *connect.Request[v1alpha1.DeleteWebhookRequest]) (*connect.Response[v1alpha1.DeleteWebhookResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.WebhookService.DeleteWebhook is not implemented"))
}

func (UnimplementedWebhookServiceHandler) ListWebhooks(context.Context, *connect.Request[v1alpha1.ListWebhooksRequest]) (*connect.Response[v1alpha1.ListWebhooksResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("buf.alpha.registry.v1alpha1.WebhookService.ListWebhooks is not implemented"))
}
