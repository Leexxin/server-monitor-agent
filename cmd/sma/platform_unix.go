//go:build !windows

package main

import (
	"context"
	"log/slog"
	"os"
)

func platformMain(run func(context.Context, func()) error) error {
	return run(context.Background(), func() {})
}
func platformArgs(args []string) ([]string, error) { return args, nil }
func platformLogger() (*slog.Logger, func(), error) {
	return slog.New(slog.NewJSONHandler(os.Stderr, nil)), func() {}, nil
}
