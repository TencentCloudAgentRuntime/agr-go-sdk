package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func run() error {
	client, err := agr.NewClient(agr.WithRegion("ap-guangzhou"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	iterator := client.Sandboxes().Iterate(agr.SandboxPageOptions{MaxResults: 20})
	defer iterator.Close()
	for {
		item, err := iterator.Next(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("%s %s\n", item.ID, item.State)
	}
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
