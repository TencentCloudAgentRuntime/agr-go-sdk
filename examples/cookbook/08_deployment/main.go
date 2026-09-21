package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
)

func main() {
	toolID := required("AGR_TOOL_ID")
	port, err := strconv.Atoi(required("AGR_DEPLOYMENT_PORT"))
	if err != nil {
		log.Fatalf("AGR_DEPLOYMENT_PORT: %v", err)
	}

	client, err := agr.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	zero, one := int64(0), int64(1)
	idle := int64(300)
	stop := agr.DeploymentIdleStop
	created, err := client.Deployments().Create(ctx, agr.CreateDeploymentOptions{
		Name:   fmt.Sprintf("agr-sdk-example-%d", time.Now().Unix()),
		ToolID: toolID,
		Scaling: &agr.CreateDeploymentScaling{
			MinInstances:   &zero,
			MaxInstances:   &one,
			MaxConcurrency: &one,
		},
		Lifecycle: &agr.CreateDeploymentLifecycle{
			IdleTimeoutSeconds: &idle,
			IdleAction:         &stop,
		},
		Tags: []agr.DeploymentTag{{Key: "example", Value: "deployment"}},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer deleteDeployment(client.Deployments(), created.ID)

	deployment, err := client.Deployments().WaitFor(ctx, created.ID, agr.DeploymentActive)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("deployment %s is %s", deployment.ID, deployment.State)

	connection, err := client.Deployments().Connect(ctx, deployment.ID, agr.DeploymentConnectOptions{Port: port})
	if err != nil {
		log.Fatal(err)
	}
	defer connection.Close() // closes local HTTP/SSE/WebSocket work; does not delete the Deployment

	response, err := connection.Do(ctx, agr.DeploymentRequest{Method: http.MethodGet, Path: "/health"})
	if err != nil {
		log.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || response.StatusCode/100 != 2 {
		log.Fatalf("health: status=%s body=%q read=%v close=%v", response.Status, body, readErr, closeErr)
	}

	// An SSE response uses the same Do method. Read it incrementally and close Body to release
	// the request. The SDK keeps the Deployment token and affinity ID internal.
	stream, err := connection.Do(ctx, agr.DeploymentRequest{
		Method: http.MethodGet,
		Path:   "/events",
		Header: http.Header{"Accept": []string{"text/event-stream"}},
	})
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(stream.Body)
	if scanner.Scan() {
		log.Printf("first SSE line: %s", scanner.Text())
	}
	if err := stream.Body.Close(); err != nil {
		log.Fatal(err)
	}

	// WebSocket uses an SDK-owned context-aware handle; no third-party type is exposed.
	socket, err := connection.DialWebSocket(ctx, agr.DeploymentWebSocketOptions{Path: "/ws"})
	if err != nil {
		log.Fatal(err)
	}
	defer socket.Close()
	if err := socket.Write(ctx, agr.DeploymentMessage{Type: agr.DeploymentTextMessage, Data: []byte("hello")}); err != nil {
		log.Fatal(err)
	}
	message, err := socket.Read(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("websocket reply: %s", message.Data)
}

func deleteDeployment(manager *agr.DeploymentManager, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := manager.Delete(ctx, id); err != nil {
		log.Printf("delete Deployment %s: %v", id, err)
		return
	}
	if err := manager.WaitDeleted(ctx, id); err != nil {
		log.Printf("confirm Deployment %s deletion: %v", id, err)
	}
}

func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}
