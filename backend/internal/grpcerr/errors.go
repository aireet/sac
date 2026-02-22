package grpcerr

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func BadRequest(msg string, errs ...error) error {
	if len(errs) > 0 && errs[0] != nil {
		msg = fmt.Sprintf("%s: %v", msg, errs[0])
	}
	return status.Error(codes.InvalidArgument, msg)
}

func Unauthorized(msg string) error {
	return status.Error(codes.Unauthenticated, msg)
}

func Forbidden(msg string) error {
	return status.Error(codes.PermissionDenied, msg)
}

func NotFound(msg string, errs ...error) error {
	if len(errs) > 0 && errs[0] != nil {
		msg = fmt.Sprintf("%s: %v", msg, errs[0])
	}
	return status.Error(codes.NotFound, msg)
}

func Conflict(msg string) error {
	return status.Error(codes.AlreadyExists, msg)
}

func Internal(msg string, err error) error {
	if err != nil {
		msg = fmt.Sprintf("%s: %v", msg, err)
	}
	return status.Error(codes.Internal, msg)
}

func Unavailable(msg string) error {
	return status.Error(codes.Unavailable, msg)
}
