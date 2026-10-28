package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
	"github.com/TencentCloudAgentRuntime/agr-go-sdk/sandbox"
)

func run() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	lifetime := 10 * time.Minute
	sb, err := sandbox.Create(ctx, sandbox.CreateOptions{
		Tool:    sandbox.ToolRef{ID: "your-tool-id"},
		Timeout: &lifetime,
	})
	if err != nil {
		return err
	}
	defer sb.Close()
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		err = errors.Join(err, sb.Delete(cleanup))
	}()

	result, err := sb.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", "printf hello"}})
	if err != nil {
		return err
	}
	fmt.Printf("exit=%d stdout=%s\n", result.ExitCode, result.Stdout)
	const path, content = "/tmp/agr-sdk-text-example.txt", "hello from sandbox"
	if _, err := sb.Files().Write(ctx, path, strings.NewReader(content), agr.WriteOptions{}); err != nil {
		return err
	}
	text, err := sb.Files().ReadText(ctx, path, agr.ReadOptions{}, 1024)
	if err != nil {
		return err
	}
	if text != content {
		return fmt.Errorf("read text differed from written content")
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
