package grpchelper

import (
	"fmt"
	"os"

	"golang.org/x/exp/slog"
	"google.golang.org/grpc/grpclog"
)

var _ grpclog.LoggerV2 = (*logger)(nil)

// NewLogger returns new grpclog Logger.
func NewLogger(l *slog.Logger) *logger {
	return &logger{l: l}
}

type logger struct {
	l *slog.Logger
}

func (l *logger) Info(args ...any) {
	l.l.Info(fmt.Sprint(args...))
}

func (l *logger) Infoln(args ...any) {
	l.l.Info(fmt.Sprint(args...))
}

func (l *logger) Infof(format string, args ...any) {
	l.l.Info(fmt.Sprintf(format, args...))
}

func (l *logger) Warning(args ...any) {
	l.l.Warn(fmt.Sprint(args...))
}

func (l *logger) Warningln(args ...any) {
	l.l.Warn(fmt.Sprint(args...))
}

func (l *logger) Warningf(format string, args ...any) {
	l.l.Warn(fmt.Sprintf(format, args...))
}

func (l *logger) Error(args ...any) {
	l.l.Error(fmt.Sprint(args...))
}

func (l *logger) Errorln(args ...any) {
	l.l.Error(fmt.Sprint(args...))
}

func (l *logger) Errorf(format string, args ...any) {
	l.l.Error(fmt.Sprintf(format, args...))
}

func (l *logger) Fatal(args ...any) {
	l.fatal(fmt.Sprint(args...))
}

func (l *logger) Fatalln(args ...any) {
	l.fatal(fmt.Sprint(args...))
}

func (l *logger) Fatalf(format string, args ...any) {
	l.fatal(fmt.Sprintf(format, args...))
}

func (l *logger) V(_ int) bool {
	return true
}

func (l *logger) fatal(msg string) {
	l.l.Error(msg)
	os.Exit(1)
}
