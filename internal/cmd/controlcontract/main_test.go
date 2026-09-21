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
	data := []byte(`{"service":"ags","version":"20250920","channels":["stable","preview"],"actions":[{"name":"Action","resource":"fixture","maturity":"stable","route":"common_raw"}],"preview_fields":[],"raw_actions":[]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readRouting(root)
	if err == nil || !strings.Contains(err.Error(), "route and raw_actions disagree") {
		t.Fatalf("error = %v, want raw route mismatch", err)
	}
}

func TestRoutingForChannelExcludesPreviewFromStable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, routingFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"service":"ags","version":"20250920","channels":["stable","preview"],"actions":[{"name":"PreviewAction","resource":"fixture","maturity":"preview","route":"official_typed"},{"name":"StableAction","resource":"fixture","maturity":"stable","route":"official_typed"}],"preview_fields":[{"object":"StableRequest","field":"PreviewValue","maturity":"preview"}],"raw_actions":[]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	routing, err := readRouting(root)
	if err != nil {
		t.Fatal(err)
	}
	stable := routingForChannel(routing, stableChannel)
	preview := routingForChannel(routing, previewChannel)
	if len(stable.Actions) != 1 || stable.Actions[0].Name != "StableAction" || len(stable.PreviewFields) != 0 {
		t.Fatalf("stable routing = %#v", stable)
	}
	if len(preview.Actions) != 2 || len(preview.PreviewFields) != 1 {
		t.Fatalf("preview routing = %#v", preview)
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
	routing := routingContract{Channels: []string{stableChannel}, Actions: []actionRoute{{Name: "Action", Resource: "fixture", Maturity: "stable", Route: "official_typed"}}}
	actions, err := manifestActions(api, routing)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Request != "Request" || actions[0].Response != "Response" || strings.Join(actions[0].Objects, ",") != "Nested,Request,Response" {
		t.Fatalf("manifest actions = %#v", actions)
	}
}

func TestValidatePreviewSupersetAcceptsDeclaredFieldAndAction(t *testing.T) {
	stable := apiContract{
		Version:  "1.0",
		Metadata: rawJSON(t, map[string]string{"service": "fixture"}),
		Actions:  map[string]json.RawMessage{"Stable": rawJSON(t, actionShape{Input: "Request", Output: "Response"})},
		Objects: map[string]json.RawMessage{
			"Request":  rawJSON(t, map[string]any{"type": "object", "members": []any{memberShape{Name: "Value", Type: "string", Member: "string"}}}),
			"Response": rawJSON(t, map[string]any{"type": "object", "members": []any{memberShape{Name: "RequestId", Type: "string", Member: "string"}}}),
		},
	}
	preview := apiContract{
		Version:  stable.Version,
		Metadata: stable.Metadata,
		Actions: map[string]json.RawMessage{
			"Stable":  stable.Actions["Stable"],
			"Preview": rawJSON(t, actionShape{Input: "PreviewRequest", Output: "PreviewResponse"}),
		},
		Objects: map[string]json.RawMessage{
			"Request": rawJSON(t, map[string]any{"type": "object", "members": []any{
				memberShape{Name: "Value", Type: "string", Member: "string"},
				memberShape{Name: "PreviewValue", Type: "string", Member: "string"},
			}}),
			"Response":        stable.Objects["Response"],
			"PreviewRequest":  rawJSON(t, map[string]any{"type": "object", "members": []any{memberShape{Name: "Input", Type: "string", Member: "string"}}}),
			"PreviewResponse": rawJSON(t, map[string]any{"type": "object", "members": []any{memberShape{Name: "Output", Type: "string", Member: "string"}}}),
		},
	}
	fields := []fieldMaturity{{Object: "Request", Field: "PreviewValue", Maturity: previewChannel}}
	if err := validatePreviewSuperset(stable, preview, fields); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePreviewSupersetRejectsStableDriftAndUndeclaredField(t *testing.T) {
	base := func(member memberShape) apiContract {
		return apiContract{
			Version:  "1.0",
			Metadata: rawJSON(t, map[string]string{"service": "fixture"}),
			Actions:  map[string]json.RawMessage{"Action": rawJSON(t, actionShape{Input: "Request", Output: "Response"})},
			Objects: map[string]json.RawMessage{
				"Request":  rawJSON(t, map[string]any{"type": "object", "members": []any{member}}),
				"Response": rawJSON(t, map[string]any{"type": "object", "members": []any{memberShape{Name: "RequestId", Type: "string", Member: "string"}}}),
			},
		}
	}
	stable := base(memberShape{Name: "Value", Type: "string", Member: "string"})
	changed := base(memberShape{Name: "Value", Type: "int", Member: "int"})
	if err := validatePreviewSuperset(stable, changed, nil); err == nil || !strings.Contains(err.Error(), "changed stable field") {
		t.Fatalf("changed field error = %v", err)
	}
	added := base(memberShape{Name: "Value", Type: "string", Member: "string"})
	added.Objects["Request"] = rawJSON(t, map[string]any{"type": "object", "members": []any{
		memberShape{Name: "Value", Type: "string", Member: "string"},
		memberShape{Name: "Extra", Type: "string", Member: "string"},
	}})
	if err := validatePreviewSuperset(stable, added, nil); err == nil || !strings.Contains(err.Error(), "undeclared field") {
		t.Fatalf("undeclared field error = %v", err)
	}
}

func TestFilterPreviewFieldsRequiresExplicitSelection(t *testing.T) {
	stable := apiContract{Objects: map[string]json.RawMessage{
		"StorageMount": rawJSON(t, map[string]any{"type": "object", "members": []any{
			memberShape{Name: "Name", Type: "string", Member: "string"},
		}}),
	}}
	preview := apiContract{Objects: map[string]json.RawMessage{
		"StorageMount": rawJSON(t, map[string]any{"type": "object", "members": []any{
			memberShape{Name: "Name", Type: "string", Member: "string"},
			memberShape{Name: "Volume", Type: "object", Member: "VolumeRef"},
		}}),
	}}
	filtered := preview
	filtered.Objects = map[string]json.RawMessage{"StorageMount": append(json.RawMessage(nil), preview.Objects["StorageMount"]...)}
	if err := filterPreviewFields(stable, &filtered, nil); err != nil {
		t.Fatal(err)
	}
	members, err := objectMembersFromContract(filtered.Objects["StorageMount"])
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].Name != "Name" {
		t.Fatalf("unselected preview field leaked: %#v", members)
	}
	selected := preview
	selected.Objects = map[string]json.RawMessage{"StorageMount": append(json.RawMessage(nil), preview.Objects["StorageMount"]...)}
	fields := []fieldMaturity{{Object: "StorageMount", Field: "Volume", Maturity: previewChannel}}
	if err := filterPreviewFields(stable, &selected, fields); err != nil {
		t.Fatal(err)
	}
	members, err = objectMembersFromContract(selected.Objects["StorageMount"])
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[1].Name != "Volume" {
		t.Fatalf("selected preview field was removed: %#v", members)
	}
}

func objectMembersFromContract(raw json.RawMessage) ([]memberShape, error) {
	var object objectShape
	err := json.Unmarshal(raw, &object)
	return object.Members, err
}

func TestReleaseChannelSeparatesStableAndPreviewVersions(t *testing.T) {
	stableRouting := routingContract{Actions: []actionRoute{{Name: "Stable", Maturity: stableChannel}}}
	if channel, err := releaseChannel("v0.2.0", stableRouting); err != nil || channel != stableChannel {
		t.Fatalf("stable release = %q, %v", channel, err)
	}
	previewRouting := routingContract{Actions: []actionRoute{{Name: "Preview", Maturity: previewChannel}}}
	if channel, err := releaseChannel("v0.3.0-preview.1", previewRouting); err != nil || channel != previewChannel {
		t.Fatalf("preview release = %q, %v", channel, err)
	}
	if _, err := releaseChannel("v0.3.0", previewRouting); err == nil || !strings.Contains(err.Error(), "stable release") {
		t.Fatalf("stable accepted preview contract: %v", err)
	}
	if _, err := releaseChannel("v0.3.0-preview.1", stableRouting); err == nil || !strings.Contains(err.Error(), "no declared preview") {
		t.Fatalf("empty preview release accepted: %v", err)
	}
}

func TestHasPreviewCapabilitiesUsesActionsAndFields(t *testing.T) {
	if hasPreviewCapabilities(routingContract{Actions: []actionRoute{{Maturity: stableChannel}}}) {
		t.Fatal("stable-only registry selected preview")
	}
	if !hasPreviewCapabilities(routingContract{Actions: []actionRoute{{Maturity: previewChannel}}}) {
		t.Fatal("preview action did not select preview")
	}
	if !hasPreviewCapabilities(routingContract{PreviewFields: []fieldMaturity{{Object: "Request", Field: "Value", Maturity: previewChannel}}}) {
		t.Fatal("preview field did not select preview")
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
