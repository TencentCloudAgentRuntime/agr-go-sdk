package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPruneContractKeepsOnlyRoutedTransitiveClosure(t *testing.T) {
	full := apiContract{
		Actions: map[string]json.RawMessage{
			"Selected": rawJSON(t, actionShape{Input: "SelectedRequest", Output: "SelectedResponse"}),
			"Future":   rawJSON(t, actionShape{Input: "FutureRequest", Output: "FutureResponse"}),
		},
		Metadata: rawJSON(t, map[string]string{"service": "fixture", "api_brief": "includes omitted APIs"}),
		Objects: map[string]json.RawMessage{
			"SelectedRequest":  rawJSON(t, objectShape{Members: []memberShape{{Name: "Nested", Type: "object", Member: "Nested"}}}),
			"SelectedResponse": rawJSON(t, objectShape{Members: []memberShape{{Name: "RequestId", Type: "string", Member: "string"}}}),
			"Nested":           rawJSON(t, objectShape{Members: []memberShape{{Name: "Value", Type: "string", Member: "string"}}}),
			"FutureRequest":    rawJSON(t, objectShape{Members: []memberShape{{Name: "Value", Type: "string", Member: "string"}}}),
			"FutureResponse":   rawJSON(t, objectShape{Members: []memberShape{{Name: "RequestId", Type: "string", Member: "string"}}}),
		},
		Version: "1.0",
	}
	routing := routingContract{Actions: []actionRoute{{Name: "Selected", Route: "official_typed"}}}

	reduced, err := pruneContract(full, routing)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduced.Actions) != 1 || len(reduced.Objects) != 3 {
		t.Fatalf("reduced contract has %d actions/%d objects, want 1/3", len(reduced.Actions), len(reduced.Objects))
	}
	if _, exists := reduced.Actions["Future"]; exists {
		t.Fatal("unrouted action was copied into the SDK contract")
	}
	if _, exists := reduced.Objects["FutureRequest"]; exists {
		t.Fatal("object outside the selected action closure was copied")
	}
	var metadata map[string]string
	if err := json.Unmarshal(reduced.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["api_brief"] != sdkAPIBrief {
		t.Fatalf("api_brief = %q, want selected-SDK description", metadata["api_brief"])
	}
}

func TestPruneContractRejectsMissingRoutedAction(t *testing.T) {
	_, err := pruneContract(apiContract{Actions: map[string]json.RawMessage{}, Metadata: rawJSON(t, map[string]string{"service": "fixture"}), Objects: map[string]json.RawMessage{}}, routingContract{Actions: []actionRoute{{Name: "Missing", Route: "official_typed"}}})
	if err == nil || !strings.Contains(err.Error(), "absent from CLI effective API") {
		t.Fatalf("error = %v, want missing allowlisted action", err)
	}
}

func TestValidateContractRejectsMissingReferencedObject(t *testing.T) {
	api := apiContract{
		Actions: map[string]json.RawMessage{"Action": rawJSON(t, actionShape{Input: "Request", Output: "MissingResponse"})},
		Objects: map[string]json.RawMessage{"Request": rawJSON(t, objectShape{Members: []memberShape{{Name: "Value", Type: "string", Member: "string"}}})},
	}
	_, err := validateContract(api, routingContract{Actions: []actionRoute{{Name: "Action", Route: "official_typed"}}})
	if err == nil || !strings.Contains(err.Error(), "missing object MissingResponse") {
		t.Fatalf("error = %v, want missing referenced object", err)
	}
}

func TestValidateContractRejectsObjectOutsideClosure(t *testing.T) {
	api := apiContract{
		Actions: map[string]json.RawMessage{"Action": rawJSON(t, actionShape{Input: "Request", Output: "Response"})},
		Objects: map[string]json.RawMessage{
			"Request":  rawJSON(t, objectShape{Members: []memberShape{{Name: "Value", Type: "string", Member: "string"}}}),
			"Response": rawJSON(t, objectShape{Members: []memberShape{{Name: "RequestId", Type: "string", Member: "string"}}}),
			"Unused":   rawJSON(t, objectShape{Members: []memberShape{{Name: "Value", Type: "string", Member: "string"}}}),
		},
	}
	_, err := validateContract(api, routingContract{Actions: []actionRoute{{Name: "Action", Route: "official_typed"}}})
	if err == nil || !strings.Contains(err.Error(), "outside routed action closure: Unused") {
		t.Fatalf("error = %v, want extra object rejection", err)
	}
}

func TestReadRoutingRejectsRawRouteMismatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, routingFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"service":"ags","version":"20250920","source_channel":"stable","actions":[{"name":"Action","resource":"fixture","maturity":"stable","route":"common_raw"}],"preview_fields":[],"raw_actions":[]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readRouting(root)
	if err == nil || !strings.Contains(err.Error(), "route and raw_actions disagree") {
		t.Fatalf("error = %v, want raw route mismatch", err)
	}
}

func TestReadRoutingRejectsPreviewActionInStableChannel(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, routingFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"service":"ags","version":"20250920","source_channel":"stable","actions":[{"name":"Action","resource":"fixture","maturity":"preview","route":"official_typed"}],"preview_fields":[],"raw_actions":[]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readRouting(root)
	if err == nil || !strings.Contains(err.Error(), "stable source cannot include preview action") {
		t.Fatalf("error = %v, want stable/preview channel rejection", err)
	}
}

func TestManifestActionsRecordPerActionClosure(t *testing.T) {
	api := apiContract{
		Actions: map[string]json.RawMessage{"Action": rawJSON(t, actionShape{Input: "Request", Output: "Response"})},
		Objects: map[string]json.RawMessage{
			"Request":  rawJSON(t, objectShape{Members: []memberShape{{Name: "Nested", Type: "object", Member: "Nested"}}}),
			"Response": rawJSON(t, objectShape{Members: []memberShape{{Name: "RequestId", Type: "string", Member: "string"}}}),
			"Nested":   rawJSON(t, objectShape{Members: []memberShape{{Name: "Value", Type: "string", Member: "string"}}}),
		},
	}
	routing := routingContract{SourceChannel: "stable", Actions: []actionRoute{{Name: "Action", Resource: "fixture", Maturity: "stable", Route: "official_typed"}}}
	actions, err := manifestActions(api, routing)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Request != "Request" || actions[0].Response != "Response" || strings.Join(actions[0].Objects, ",") != "Nested,Request,Response" {
		t.Fatalf("manifest actions = %#v", actions)
	}
}

func TestCompactJSONUsesStableTrailingNewline(t *testing.T) {
	first, err := compactJSON([]byte("{\n  \"b\": 2, \"a\": 1\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := compactJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "{\"b\":2,\"a\":1}\n" || string(second) != string(first) {
		t.Fatalf("compaction is not stable: first=%q second=%q", first, second)
	}
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
