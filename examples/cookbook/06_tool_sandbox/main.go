package main

import (
	"context"
	"fmt"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func run() error {
	client, err := agr.NewClient(agr.WithRegion("ap-guangzhou"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	created, err := client.Tools().Create(ctx, agr.CreateToolOptions{
		Name:    fmt.Sprintf("agr-sdk-example-%d", time.Now().Unix()),
		Type:    agr.ToolCodeInterpreter,
		Network: agr.ToolNetworkConfiguration{Mode: agr.ToolNetworkSandbox},
	})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Minute)
		defer stop()
		_ = client.Tools().Delete(cleanup, created.ToolID)
	}()
	if _, err = client.Tools().WaitFor(ctx, created.ToolID, agr.ToolActive); err != nil {
		return err
	}
	lifetime := 5 * time.Minute
	sb, err := client.Sandboxes().Create(ctx, agr.CreateOptions{Tool: agr.ToolRef{ID: created.ToolID}, Timeout: &lifetime})
	if err != nil {
		return err
	}
	defer sb.Close()
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Minute)
		defer stop()
		_ = sb.Delete(cleanup)
	}()
	result, err := sb.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", "printf tool-sandbox-ok"}, User: agr.Root})
	if err != nil {
		return err
	}
	fmt.Println(string(result.Stdout))
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
