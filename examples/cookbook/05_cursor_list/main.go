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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	opts := agr.SandboxPageOptions{MaxResults: 20, NeedTotalCount: true}
	for {
		page, err := client.Sandboxes().ListPage(ctx, opts)
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			fmt.Printf("%s %s\n", item.ID, item.State)
		}
		if page.NextToken == "" {
			return nil
		}
		opts.NextToken = page.NextToken
	}
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
