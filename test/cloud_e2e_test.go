package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
	shortcut "github.com/TencentCloudAgentRuntime/agr-go-sdk/sandbox"
)

func TestDefaultShortcutCloudJourney(t *testing.T) {
	environment := requireCloudEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	timeout := 5 * time.Minute
	client, err := agr.DefaultClient()
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := shortcut.Create(ctx, shortcut.CreateOptions{
		Tool:    shortcut.ToolRef{ID: environment.toolID},
		Timeout: &timeout,
		Metadata: map[string]string{
			"agr-sdk-e2e": "default-shortcut",
		},
	})
	if err != nil {
		cleanupAcceptedCreate(t, client.Sandboxes(), err)
		t.Fatal(err)
	}
	defer deleteAndConfirm(t, client.Sandboxes(), sandbox)
	result, err := sandbox.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", "printf default-ok"}, User: agr.Root})
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "default-ok" {
		t.Fatalf("default shortcut command = %+v, %v", result, err)
	}
}

func TestExplicitClientCloudJourney(t *testing.T) {
	environment := requireCloudEnvironment(t)
	client := newCloudClient(t, environment)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	timeout := 5 * time.Minute
	sandbox, err := client.Sandboxes().Create(ctx, agr.CreateOptions{
		Tool:     agr.ToolRef{ID: environment.toolID},
		Timeout:  &timeout,
		AuthMode: agr.SandboxAuthDefault,
		Env: map[string]string{
			"AGS_SDK_E2E_EMPTY": "",
			"AGS_SDK_E2E_VALUE": "submitted",
		},
		Metadata: map[string]string{
			"agr-sdk-e2e": "explicit-client",
		},
	})
	if err != nil {
		cleanupAcceptedCreate(t, client.Sandboxes(), err)
		t.Fatal(err)
	}
	defer deleteAndConfirm(t, client.Sandboxes(), sandbox)

	if _, err := sandbox.Update(ctx, agr.UpdateOptions{Timeout: &timeout, MetadataUpsert: map[string]string{"agr-sdk-e2e-updated": ""}}); err != nil {
		t.Fatal(err)
	}
	connected, err := client.Sandboxes().Connect(ctx, sandbox.ID(), agr.ConnectOptions{Timeout: &timeout})
	if err != nil {
		t.Fatal(err)
	}
	defer connected.Close()
	if connected.ID() != sandbox.ID() {
		t.Fatalf("connected ID = %q; want %q", connected.ID(), sandbox.ID())
	}
	if _, err := connected.GetHost(8080); err != nil {
		t.Fatal(err)
	}

	t.Run("files-and-watch", func(t *testing.T) {
		directory := "/tmp/agr-sdk-e2e"
		_, _ = sandbox.Files().MakeDir(ctx, directory, agr.MakeDirOptions{User: agr.Root})
		watch, err := sandbox.Files().Watch(ctx, directory, agr.WatchOptions{Buffer: 8, User: agr.Root})
		if err != nil {
			t.Fatal(err)
		}
		defer watch.Close()
		path := directory + "/input.txt"
		written, err := sandbox.Files().Write(ctx, path, strings.NewReader("streamed-content"), agr.WriteOptions{User: agr.Root})
		if err != nil || written.Path == "" {
			t.Fatalf("Write = %+v, %v", written, err)
		}
		exists, err := sandbox.Files().Exists(ctx, path, agr.ExistsOptions{User: agr.Root})
		if err != nil || !exists {
			t.Fatalf("Exists = %v, %v", exists, err)
		}
		reader, err := sandbox.Files().Read(ctx, path, agr.ReadOptions{User: agr.Root})
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || string(content) != "streamed-content" {
			t.Fatalf("Read = %q, %v, close=%v", content, readErr, closeErr)
		}
		moved := directory + "/moved.txt"
		if _, err := sandbox.Files().Move(ctx, path, moved, agr.MoveOptions{User: agr.Root}); err != nil {
			t.Fatal(err)
		}
		select {
		case event, ok := <-watch.Events():
			if !ok || event.Path == "" {
				t.Fatalf("Watch event = %+v, open=%v, err=%v", event, ok, watch.Err())
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Watch produced no event within 30 seconds")
		}
		if err := sandbox.Files().Remove(ctx, moved, agr.RemoveOptions{User: agr.Root}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("commands-list-connect", func(t *testing.T) {
		result, err := sandbox.Commands().Run(ctx, "sh", agr.CommandOptions{Args: []string{"-lc", "printf stdout; printf stderr >&2"}, User: agr.Root})
		if err != nil || result.ExitCode != 0 || string(result.Stdout) != "stdout" || string(result.Stderr) != "stderr" {
			t.Fatalf("Run = %+v, %v", result, err)
		}
		handle, err := sandbox.Commands().Start(ctx, "sh", agr.StartOptions{Args: []string{"-lc", "sleep 3; printf reconnected"}, User: agr.Root})
		if err != nil {
			t.Fatal(err)
		}
		pid := handle.PID()
		processes, err := sandbox.Commands().List(ctx, agr.CommandListOptions{User: agr.Root})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, process := range processes {
			found = found || process.PID == pid
		}
		if !found {
			t.Fatalf("started PID %d not returned by Commands.List", pid)
		}
		if err := handle.Close(); err != nil {
			t.Fatal(err)
		}
		reconnected, err := sandbox.Commands().Connect(ctx, pid, agr.ConnectCommandOptions{User: agr.Root})
		if err != nil {
			t.Fatal(err)
		}
		defer reconnected.Close()
		result, err = reconnected.Wait(ctx)
		if err != nil || result.ExitCode != 0 || !bytes.Contains(result.Stdout, []byte("reconnected")) {
			t.Fatalf("connected command = %+v, %v", result, err)
		}
	})

	t.Run("pty", func(t *testing.T) {
		session, err := sandbox.PTY().Open(ctx, agr.PTYOptions{Command: "sh", Args: []string{"-lc", "printf pty-ok"}, Cols: 80, Rows: 24, User: agr.Root})
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		outputDone := make(chan []byte, 1)
		go func() {
			var output []byte
			for event := range session.Events() {
				if event.Type == agr.PTYOutput {
					output = append(output, event.Data...)
				}
			}
			outputDone <- output
		}()
		exit, err := session.Wait(ctx)
		if err != nil || exit.Code != 0 {
			t.Fatalf("PTY exit = %+v, %v", exit, err)
		}
		output := <-outputDone
		if !bytes.Contains(output, []byte("pty-ok")) {
			t.Fatalf("PTY output = %q", output)
		}
	})

	t.Run("metrics-window", func(t *testing.T) {
		end := time.Now().UTC()
		start := end.Add(-15 * time.Minute)
		period := time.Minute
		result, err := sandbox.Metrics().Get(ctx, agr.MetricsQuery{Start: &start, End: &end, Period: &period})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Series)+len(result.Unavailable) != 10 {
			t.Fatalf("Metrics result = %+v", result)
		}
	})

}

func TestCodeCloudJourney(t *testing.T) {
	environment := requireCloudEnvironment(t)
	client := newCloudClient(t, environment)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	timeout := 5 * time.Minute
	sandbox, err := client.Sandboxes().Create(ctx, agr.CreateOptions{
		Tool:     agr.ToolRef{ID: environment.codeToolID},
		Timeout:  &timeout,
		Metadata: map[string]string{"agr-sdk-e2e": "code-context"},
	})
	if err != nil {
		cleanupAcceptedCreate(t, client.Sandboxes(), err)
		t.Fatal(err)
	}
	defer deleteAndConfirm(t, client.Sandboxes(), sandbox)
	connected, err := client.Sandboxes().Connect(ctx, sandbox.ID(), agr.ConnectOptions{Timeout: &timeout})
	if err != nil {
		t.Fatal(err)
	}
	defer connected.Close()

	managed, err := sandbox.Code().CreateContext(ctx, agr.CreateCodeContextOptions{Language: "python"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sandbox.Code().Run(ctx, "counter = 1", agr.RunCodeOptions{Context: managed}, agr.CodeCallbacks{}); err != nil {
		var failure *agr.Error
		if errors.As(err, &failure) && failure.Cause != nil {
			t.Fatalf("%v; diagnostic=%v", err, failure.Cause)
		}
		t.Fatal(err)
	}
	execution, err := sandbox.Code().Run(ctx, "counter += 1; print(counter)", agr.RunCodeOptions{Context: managed}, agr.CodeCallbacks{})
	if err != nil || execution.Error != nil || !strings.Contains(strings.Join(execution.Stdout, ""), "2") {
		t.Fatalf("managed context execution = %+v, %v", execution, err)
	}
	external, err := agr.NewExternalCodeContextRef(managed.ID())
	if err != nil {
		t.Fatal(err)
	}
	execution, err = connected.Code().Run(ctx, "print(counter)", agr.RunCodeOptions{Context: external}, agr.CodeCallbacks{})
	if err != nil || execution.Error != nil || !strings.Contains(strings.Join(execution.Stdout, ""), "2") {
		t.Fatalf("external context after reconnect = %+v, %v", execution, err)
	}

	if _, err := sandbox.Pause(ctx, agr.PauseOptions{Mode: agr.PauseDisk}); err != nil {
		t.Fatal(err)
	}
	resumeTimeout := 30 * time.Second
	if _, err := sandbox.Resume(ctx, agr.ResumeOptions{Timeout: &resumeTimeout}); err != nil {
		assertCloudBoundaryRejection(t, "Resume(30s)", err)
		cloudTimeout := 5 * time.Minute
		if _, retryErr := sandbox.Resume(ctx, agr.ResumeOptions{Timeout: &cloudTimeout}); retryErr != nil {
			t.Fatalf("Resume(300s) after an explicit 30s service rejection: %v", retryErr)
		}
	}
	_, err = sandbox.Code().Run(ctx, "print('stale')", agr.RunCodeOptions{Context: managed}, agr.CodeCallbacks{})
	var failure *agr.Error
	if !errors.As(err, &failure) || failure.Code != agr.Conflict || failure.Reason != "CODE_CONTEXT_INVALIDATED" {
		t.Fatalf("stale managed context error = %v", err)
	}
	external, err = agr.NewExternalCodeContextRef(managed.ID())
	if err != nil {
		t.Fatal(err)
	}
	execution, externalErr := sandbox.Code().Run(ctx, "print('after-resume')", agr.RunCodeOptions{Context: external}, agr.CodeCallbacks{})
	if externalErr == nil && execution.Error == nil {
		t.Log("service accepted an external context after Pause/Resume")
	} else {
		t.Logf("service rejected an external context after Pause/Resume: %v, remote=%+v", externalErr, executionError(execution))
	}
}

func executionError(execution *agr.CodeExecution) *agr.CodeExecutionError {
	if execution == nil {
		return nil
	}
	return execution.Error
}

func TestCreateThirtySecondBoundaryPreservesServiceDecision(t *testing.T) {
	environment := requireCloudEnvironment(t)
	client := newCloudClient(t, environment)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	timeout := 30 * time.Second
	sandbox, err := client.Sandboxes().Create(ctx, agr.CreateOptions{
		Tool:     agr.ToolRef{ID: environment.toolID},
		Timeout:  &timeout,
		Metadata: map[string]string{"agr-sdk-e2e": "create-30-second-boundary"},
	})
	if err != nil {
		cleanupAcceptedCreate(t, client.Sandboxes(), err)
		assertCloudBoundaryRejection(t, "Create(30s)", err)
		return
	}
	defer deleteAndConfirm(t, client.Sandboxes(), sandbox)
	if sandbox.ID() == "" {
		t.Fatal(fmt.Errorf("Create(30s) returned an empty sandbox ID"))
	}
}

func assertCloudBoundaryRejection(t *testing.T, operation string, err error) {
	t.Helper()
	var failure *agr.Error
	if !errors.As(err, &failure) || failure.Code != agr.InvalidArgument || failure.RequestID == "" {
		t.Fatalf("%s returned an unexpected error: %v", operation, err)
	}
	if failure.Reason == "" {
		t.Fatalf("%s lost the service reason", operation)
	}
	t.Logf("%s was accepted by the SDK and rejected by the deployed Cloud policy (%s)", operation, failure.Reason)
}
