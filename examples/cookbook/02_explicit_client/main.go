package main

import (
	"context"
	"fmt"
	"os"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func run() error {
	options := []agr.ClientOption{
		agr.WithRegion("ap-guangzhou"),
		agr.WithCredential(agr.CloudCredential{
			SecretID:  os.Getenv("TENCENTCLOUD_SECRET_ID"),
			SecretKey: os.Getenv("TENCENTCLOUD_SECRET_KEY"),
		}),
	}
	if domain := os.Getenv("AGR_SANDBOX_DOMAIN"); domain != "" {
		options = append(options, agr.WithSandboxDomain(domain))
	}
	client, err := agr.NewClient(options...)
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
	if id := os.Getenv("AGR_SANDBOX_ID"); id != "" {
		sb, err := client.Sandboxes().Connect(ctx, id)
		if err != nil {
			return err
		}
		defer sb.Close()
		host, err := sb.GetHost(8080)
		if err != nil {
			return err
		}
		fmt.Println(host) // The Tool must actually expose port 8080 for this host to be reachable.
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
