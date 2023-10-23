package grpchelper

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errInternal = status.Error(codes.Internal, "internal error")
