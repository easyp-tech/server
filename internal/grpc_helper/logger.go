package grpc_helper

import (
	"fmt"
	"log/slog"

	"google.golang.org/grpc/grpclog"
)

var _ grpclog.LoggerV2 = &logger{}

// NewLogger returns new grpclog Logger.
func NewLogger(l *slog.Logger) grpclog.LoggerV2 {
	return &logger{l: l}
}

type logger struct {
	l *slog.Logger
}

func (l *logger) Info(args ...any) {
	l.l.Info("", args...)
}

func (l *logger) Infoln(args ...any) {
	l.l.Info("", args...)
}

func (l *logger) Infof(format string, args ...any) {
	l.l.Info(format, args...)
}

func (l *logger) Warning(args ...any) {
	l.l.Warn("", args...)
}

func (l *logger) Warningln(args ...any) {
	l.l.Warn("", args...)
}

func (l *logger) Warningf(format string, args ...any) {
	l.l.Warn(format, args...)
}

func (l *logger) Error(args ...any) {
	l.l.Error("", args...)
}

func (l *logger) Errorln(args ...any) {
	l.l.Error("", args...)
}

func (l *logger) Errorf(format string, args ...any) {
	l.l.Error(format, args...)
}

func (l *logger) Fatal(args ...any) {
	panic(fmt.Sprint(args...)) // todo
}

func (l *logger) Fatalln(args ...any) {
	panic(fmt.Sprint(args...)) // todo
}

func (l *logger) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...)) // todo
}

func (l *logger) V(level int) bool {
	return true // todo
}
