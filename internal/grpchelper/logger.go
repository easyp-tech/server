package grpchelper

import (
	"fmt"
	"os"

	"golang.org/x/exp/slog"
	"google.golang.org/grpc/grpclog"
)

var _ grpclog.LoggerV2 = (*grpcLogger)(nil)

// NewLogger returns new grpclog Logger.
func NewLogger(l *slog.Logger) *grpcLogger {
	return &grpcLogger{l: l}
}

type grpcLogger struct {
	l *slog.Logger
}

func (l *grpcLogger) Info(args ...any) {
	l.l.Info(fmt.Sprint(args...))
}

func (l *grpcLogger) Infoln(args ...any) {
	l.l.Info(fmt.Sprint(args...))
}

func (l *grpcLogger) Infof(format string, args ...any) {
	l.l.Info(fmt.Sprintf(format, args...))
}

func (l *grpcLogger) Warning(args ...any) {
	l.l.Warn(fmt.Sprint(args...))
}

func (l *grpcLogger) Warningln(args ...any) {
	l.l.Warn(fmt.Sprint(args...))
}

func (l *grpcLogger) Warningf(format string, args ...any) {
	l.l.Warn(fmt.Sprintf(format, args...))
}

func (l *grpcLogger) Error(args ...any) {
	l.l.Error(fmt.Sprint(args...))
}

func (l *grpcLogger) Errorln(args ...any) {
	l.l.Error(fmt.Sprint(args...))
}

func (l *grpcLogger) Errorf(format string, args ...any) {
	l.l.Error(fmt.Sprintf(format, args...))
}

func (l *grpcLogger) Fatal(args ...any) {
	l.fatal(fmt.Sprint(args...))
}

func (l *grpcLogger) Fatalln(args ...any) {
	l.fatal(fmt.Sprint(args...))
}

func (l *grpcLogger) Fatalf(format string, args ...any) {
	l.fatal(fmt.Sprintf(format, args...))
}

func (l *grpcLogger) V(_ int) bool {
	return true
}

func (l *grpcLogger) fatal(msg string) {
	l.l.Error(msg)
	os.Exit(1)
}
