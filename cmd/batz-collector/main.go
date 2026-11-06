package main

import (
	"batz/collector/plugins"
	"batz/internal/buildinfo"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/warpstreamlabs/bento/public/components/pure"
	"github.com/warpstreamlabs/bento/public/service"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, plugins.ErrUnsupportedShape) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		identity := buildinfo.Current()
		return fmt.Errorf("batz-collector %s (%s, %s): usage: batz-collector NATIVE_CONFIG CONTEXT_JSON", identity.Version, identity.SourceSHA, identity.Target)
	}
	b, err := os.ReadFile(os.Args[2])
	if err != nil {
		return err
	}
	var req plugins.Request
	if err = json.Unmarshal(b, &req); err != nil {
		return err
	}
	job := &plugins.Job{Request: req}
	if err = plugins.Register(job); err != nil {
		return err
	}
	config, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	builder := service.NewStreamBuilder()
	builder.SetThreads(1)
	if err = builder.SetYAML(string(config)); err != nil {
		return err
	}
	stream, err := builder.Build()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, time.Duration(req.Limits.CollectionSeconds)*time.Second)
	defer timeout()
	if err = stream.Run(ctx); err != nil {
		_ = stream.StopWithin(time.Second)
		return err
	}
	if err = job.Err(); err != nil {
		return err
	}
	return job.Finish()
}
