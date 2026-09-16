package main

import (
	"context"
	"os"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func run() error {
	client, err := agr.NewClient(
		agr.WithRegion("ap-guangzhou"),
		agr.WithCredential(agr.CloudCredential{
			SecretID:  os.Getenv("TENCENTCLOUD_SECRET_ID"),
			SecretKey: os.Getenv("TENCENTCLOUD_SECRET_KEY"),
		}),
	)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	page, err := client.Sandboxes().List(ctx, agr.SandboxListOptions{Limit: 20})
	if err != nil {
		return err
	}
	for _, info := range page.Items {
		_, _ = info.ID, info.State
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
