package main

import (
	"context"
	"fmt"
	"os"
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
	registryType := agr.ImageRegistryType(os.Getenv("AGR_PRECACHE_REGISTRY_TYPE"))
	created, err := client.PreCacheImages().Create(ctx, agr.CreatePreCacheImageOptions{
		Image:        os.Getenv("AGR_PRECACHE_IMAGE"),
		RegistryType: registryType,
	})
	if err != nil {
		return err
	}
	for {
		info, err := client.PreCacheImages().Get(ctx, created.Ref)
		if err != nil {
			return err
		}
		fmt.Printf("status=%s message=%s\n", info.Status, info.Message)
		if info.Status == "Success" || info.Status == "SUCCESS" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
