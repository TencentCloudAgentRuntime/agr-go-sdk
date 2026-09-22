// Command controlcontract synchronizes and verifies the public SDK's stable and
// preview control-plane contracts against a pinned ags-cli checkout.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/TencentCloudAgentRuntime/agr-go-sdk/internal/controlplane"
)

const (
	routingFile      = "contracts/control-plane.json"
	sourceRepository = "https://github.com/TencentCloudAgentRuntime/ags-cli"
	sdkAPIBrief      = "Agent Sandbox 沙箱管理与调用"
	stableChannel    = "stable"
	previewChannel   = "preview"
	allChannels      = "all"
)

var releaseVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-preview\.[1-9][0-9]*)?$`)

type actionRoute struct {
	Name     string `json:"name"`
	Resource string `json:"resource"`
	Maturity string `json:"maturity"`
	Route    string `json:"route"`
}

type fieldMaturity struct {
	Object   string `json:"object"`
	Field    string `json:"field"`
	Maturity string `json:"maturity"`
}

type routingContract struct {
	Service       string          `json:"service"`
	Version       string          `json:"version"`
	Channels      []string        `json:"channels"`
	Actions       []actionRoute   `json:"actions"`
	PreviewFields []fieldMaturity `json:"preview_fields"`
	CommonActions []string        `json:"common_actions"`
}

type manifestAction struct {
	Name     string   `json:"name"`
	Resource string   `json:"resource"`
	Maturity string   `json:"maturity"`
	Route    string   `json:"route"`
	Request  string   `json:"request"`
	Response string   `json:"response"`
	Objects  []string `json:"objects"`
}

type manifest struct {
	SchemaVersion            int              `json:"schema_version"`
	SourceRepository         string           `json:"source_repository"`
	SourceRevision           string           `json:"source_revision"`
	SourceChannel            string           `json:"source_channel"`
	APIVersion               string           `json:"api_version"`
	SourcePath               string           `json:"source_path"`
	SourceAPIJSONSHA256      string           `json:"source_schema_sha256"`
	SourceAPIPatchJSONSHA256 string           `json:"source_patch_sha256"`
	SourceEffectiveSHA256    string           `json:"source_effective_sha256"`
	SDKEffectiveSHA256       string           `json:"sdk_effective_sha256"`
	SDKEffectiveActions      int              `json:"sdk_effective_actions"`
	SDKEffectiveObjects      int              `json:"sdk_effective_objects"`
	FieldMaturityDefault     string           `json:"field_maturity_default"`
	PreviewFields            []fieldMaturity  `json:"preview_fields"`
	Actions                  []manifestAction `json:"actions"`
}

type apiContract struct {
	Actions  map[string]json.RawMessage `json:"actions"`
	Metadata json.RawMessage            `json:"metadata"`
	Objects  map[string]json.RawMessage `json:"objects"`
	Version  string                     `json:"version"`
}

type actionShape struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type objectShape struct {
	Members []memberShape `json:"members"`
}

type memberShape struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Member         string `json:"member"`
	Required       *bool  `json:"required,omitempty"`
	OutputRequired *bool  `json:"output_required,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "controlcontract:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: controlcontract <sync|verify|release-check|current-channel> [flags]")
	}
	switch args[0] {
	case "sync":
		return runSync(args[1:], stdout)
	case "verify":
		return runVerify(args[1:], stdout)
	case "release-check":
		return runReleaseCheck(args[1:], stdout)
	case "current-channel":
		return runCurrentChannel(args[1:], stdout)
	default:
		return fmt.Errorf("unknown subcommand %q (allowed: sync, verify, release-check, current-channel)", args[0])
	}
}

func runSync(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cliDir := fs.String("cli-dir", "", "path to a pinned ags-cli checkout")
	revision := fs.String("revision", "", "required ags-cli revision")
	root := fs.String("root", ".", "agr-go-sdk repository root")
	channel := fs.String("channel", allChannels, "contract channel: stable, preview, or all")
	check := fs.Bool("check", false, "compare generated files without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *cliDir == "" || *revision == "" {
		return errors.New("sync requires --cli-dir and --revision and accepts no positional arguments")
	}
	routing, err := readRouting(*root)
	if err != nil {
		return err
	}
	channels, err := selectChannels(*channel, routing.Channels)
	if err != nil {
		return err
	}
	generationChannels := append([]string(nil), channels...)
	if contains(generationChannels, previewChannel) && !contains(generationChannels, stableChannel) {
		generationChannels = append([]string{stableChannel}, generationChannels...)
	}
	generated := make(map[string]generatedContract, len(generationChannels))
	for _, selected := range generationChannels {
		channelRouting := routingForChannel(routing, selected)
		value, generateErr := generateFromCLI(*cliDir, *revision, selected, channelRouting)
		if generateErr != nil {
			return generateErr
		}
		generated[selected] = value
	}
	if stable, stableOK := generated[stableChannel]; stableOK {
		if preview, previewOK := generated[previewChannel]; previewOK {
			if err := validatePreviewSuperset(stable.contract, preview.contract, routing.PreviewFields); err != nil {
				return err
			}
		}
	}
	for _, selected := range channels {
		value := generated[selected]
		files := []struct {
			path string
			data []byte
		}{
			{path: filepath.Join(*root, contractPath(routing, selected, "effective.json")), data: value.effective},
			{path: filepath.Join(*root, contractPath(routing, selected, "MANIFEST.json")), data: value.manifest},
		}
		for _, file := range files {
			if *check {
				current, readErr := os.ReadFile(file.path)
				if readErr != nil {
					return fmt.Errorf("read generated file %s: %w", file.path, readErr)
				}
				if !bytes.Equal(current, file.data) {
					return fmt.Errorf("generated control contract is stale: %s", file.path)
				}
				continue
			}
			if err := writeAtomic(file.path, file.data); err != nil {
				return err
			}
		}
	}
	verb := "synchronized"
	if *check {
		verb = "current"
	}
	for _, selected := range channels {
		value := generated[selected]
		fmt.Fprintf(stdout, "%s control contract %s: %d actions, %d objects, revision %s\n", selected, verb, value.actions, value.objects, *revision)
	}
	return nil
}

func runVerify(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", ".", "agr-go-sdk repository root")
	channel := fs.String("channel", allChannels, "contract channel: stable, preview, or all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("verify accepts no positional arguments")
	}
	routing, err := readRouting(*root)
	if err != nil {
		return err
	}
	channels, err := selectChannels(*channel, routing.Channels)
	if err != nil {
		return err
	}
	if err := compareImplementationRoutes(routing); err != nil {
		return err
	}
	verified := make(map[string]apiContract, len(channels))
	for _, selected := range channels {
		contract, verifyErr := verifyChannel(*root, selected, routing, stdout)
		if verifyErr != nil {
			return verifyErr
		}
		verified[selected] = contract
	}
	if _, previewOK := verified[previewChannel]; previewOK {
		if _, stableOK := verified[stableChannel]; !stableOK {
			stable, verifyErr := verifyChannel(*root, stableChannel, routing, io.Discard)
			if verifyErr != nil {
				return verifyErr
			}
			verified[stableChannel] = stable
		}
	}
	if stable, stableOK := verified[stableChannel]; stableOK {
		if preview, previewOK := verified[previewChannel]; previewOK {
			if err := validatePreviewSuperset(stable, preview, routing.PreviewFields); err != nil {
				return err
			}
			fmt.Fprintln(stdout, "preview contract verified as a stable superset")
		}
	}
	return nil
}

func verifyChannel(root, channel string, routing routingContract, stdout io.Writer) (apiContract, error) {
	selected := routingForChannel(routing, channel)
	manifestPath := filepath.Join(root, contractPath(routing, channel, "MANIFEST.json"))
	effectivePath := filepath.Join(root, contractPath(routing, channel, "effective.json"))
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return apiContract{}, fmt.Errorf("read %s manifest: %w", channel, err)
	}
	var recorded manifest
	if err := json.Unmarshal(manifestData, &recorded); err != nil {
		return apiContract{}, fmt.Errorf("decode %s manifest: %w", channel, err)
	}
	effective, err := os.ReadFile(effectivePath)
	if err != nil {
		return apiContract{}, fmt.Errorf("read %s effective snapshot: %w", channel, err)
	}
	if got := digest(effective); got != recorded.SDKEffectiveSHA256 {
		return apiContract{}, fmt.Errorf("%s effective snapshot SHA-256 is %s, expected %s", channel, got, recorded.SDKEffectiveSHA256)
	}
	for name, value := range map[string]string{
		"source api.json":       recorded.SourceAPIJSONSHA256,
		"source api.patch.json": recorded.SourceAPIPatchJSONSHA256,
		"source effective API":  recorded.SourceEffectiveSHA256,
	} {
		if !validDigest(value) {
			return apiContract{}, fmt.Errorf("%s %s SHA-256 is invalid", channel, name)
		}
	}
	wantSourcePath := filepath.ToSlash(filepath.Join("api", routing.Service, "v"+routing.Version))
	if recorded.SchemaVersion != 2 || recorded.SourceRepository != sourceRepository || recorded.SourceRevision == "" || recorded.SourcePath != wantSourcePath || recorded.SourceChannel != channel || recorded.APIVersion != routing.Version || recorded.FieldMaturityDefault != stableChannel {
		return apiContract{}, fmt.Errorf("%s manifest source metadata is incomplete", channel)
	}
	var api apiContract
	if err := json.Unmarshal(effective, &api); err != nil {
		return apiContract{}, fmt.Errorf("decode %s effective snapshot: %w", channel, err)
	}
	closure, err := validateContract(api, selected)
	if err != nil {
		return apiContract{}, err
	}
	if len(api.Actions) != recorded.SDKEffectiveActions || len(closure) != recorded.SDKEffectiveObjects {
		return apiContract{}, fmt.Errorf("%s manifest counts are %d actions/%d objects, snapshot has %d/%d", channel, recorded.SDKEffectiveActions, recorded.SDKEffectiveObjects, len(api.Actions), len(closure))
	}
	wantActions, err := manifestActions(api, selected)
	if err != nil {
		return apiContract{}, err
	}
	if !reflect.DeepEqual(recorded.Actions, wantActions) || !reflect.DeepEqual(recorded.PreviewFields, selected.PreviewFields) {
		return apiContract{}, fmt.Errorf("%s manifest action or field maturity metadata is stale", channel)
	}
	typed, common := routeCounts(selected)
	fmt.Fprintf(stdout, "%s control contract verified: %d actions, %d objects, %d typed, %d common client\n", channel, len(api.Actions), len(closure), typed, common)
	return api, nil
}

type generatedContract struct {
	effective, manifest []byte
	actions, objects    int
	contract            apiContract
}

func generateFromCLI(cliDir, revision, channel string, routing routingContract) (generatedContract, error) {
	head, err := commandOutput(cliDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return generatedContract{}, fmt.Errorf("read ags-cli revision: %w", err)
	}
	if strings.TrimSpace(string(head)) != revision {
		return generatedContract{}, fmt.Errorf("ags-cli revision is %s, expected %s", strings.TrimSpace(string(head)), revision)
	}
	relativeAPIPath := filepath.Join("api", routing.Service, "v"+routing.Version)
	status, err := commandOutput(cliDir, "git", "status", "--porcelain")
	if err != nil {
		return generatedContract{}, fmt.Errorf("inspect ags-cli contract worktree: %w", err)
	}
	if strings.TrimSpace(string(status)) != "" {
		return generatedContract{}, fmt.Errorf("ags-cli worktree is not clean: %s", strings.TrimSpace(string(status)))
	}
	apiPath := filepath.Join(cliDir, relativeAPIPath)
	apiData, err := os.ReadFile(filepath.Join(apiPath, "api.json"))
	if err != nil {
		return generatedContract{}, fmt.Errorf("read source api.json: %w", err)
	}
	patchData, err := os.ReadFile(filepath.Join(apiPath, "api.patch.json"))
	if err != nil {
		return generatedContract{}, fmt.Errorf("read source api.patch.json: %w", err)
	}
	selected := apiData
	if channel == previewChannel {
		selected, err = commandOutput(cliDir, "go", "run", "./cmd/internal/apipatch", "render", "--api", relativeAPIPath)
		if err != nil {
			return generatedContract{}, fmt.Errorf("render CLI preview API: %w", err)
		}
	}
	compact, err := compactJSON(selected)
	if err != nil {
		return generatedContract{}, fmt.Errorf("compact CLI effective API: %w", err)
	}
	var full apiContract
	if err := json.Unmarshal(selected, &full); err != nil {
		return generatedContract{}, fmt.Errorf("decode CLI %s API: %w", channel, err)
	}
	if channel == previewChannel {
		var base apiContract
		if err := json.Unmarshal(apiData, &base); err != nil {
			return generatedContract{}, fmt.Errorf("decode CLI stable API: %w", err)
		}
		stable, err := pruneContract(base, routingForChannel(routing, stableChannel))
		if err != nil {
			return generatedContract{}, fmt.Errorf("derive stable SDK contract: %w", err)
		}
		if err := filterPreviewFields(stable, &full, routing.PreviewFields); err != nil {
			return generatedContract{}, err
		}
	}
	reduced, err := pruneContract(full, routing)
	if err != nil {
		return generatedContract{}, err
	}
	effective, err := marshalFile(reduced)
	if err != nil {
		return generatedContract{}, err
	}
	actions, err := manifestActions(reduced, routing)
	if err != nil {
		return generatedContract{}, err
	}
	recorded := manifest{
		SchemaVersion: 2, SourceRepository: sourceRepository, SourceRevision: revision,
		SourceChannel: channel, APIVersion: routing.Version,
		SourcePath:          filepath.ToSlash(relativeAPIPath),
		SourceAPIJSONSHA256: digest(apiData), SourceAPIPatchJSONSHA256: digest(patchData),
		SourceEffectiveSHA256: digest(compact), SDKEffectiveSHA256: digest(effective),
		SDKEffectiveActions: len(reduced.Actions), SDKEffectiveObjects: len(reduced.Objects),
		FieldMaturityDefault: stableChannel, PreviewFields: routing.PreviewFields, Actions: actions,
	}
	manifestData, err := marshalFile(recorded)
	if err != nil {
		return generatedContract{}, err
	}
	return generatedContract{effective: effective, manifest: manifestData, actions: len(reduced.Actions), objects: len(reduced.Objects), contract: reduced}, nil
}

func runReleaseCheck(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("release-check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", ".", "agr-go-sdk repository root")
	version := fs.String("version", "", "release version, for example v0.2.0 or v0.3.0-preview.1")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *version == "" {
		return errors.New("release-check requires --version and accepts no positional arguments")
	}
	routing, err := readRouting(*root)
	if err != nil {
		return err
	}
	channel, err := releaseChannel(*version, routing)
	if err != nil {
		return err
	}
	if err := compareImplementationRoutes(routing); err != nil {
		return err
	}
	selected, err := verifyChannel(*root, channel, routing, io.Discard)
	if err != nil {
		return err
	}
	if channel == previewChannel {
		stable, err := verifyChannel(*root, stableChannel, routing, io.Discard)
		if err != nil {
			return err
		}
		if err := validatePreviewSuperset(stable, selected, routing.PreviewFields); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "%s is eligible for the %s contract channel\n", *version, channel)
	return nil
}

func releaseChannel(version string, routing routingContract) (string, error) {
	if !releaseVersionPattern.MatchString(version) {
		return "", fmt.Errorf("release version %q must be vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-preview.N", version)
	}
	hasPreview := hasPreviewCapabilities(routing)
	if strings.Contains(version, "-preview.") {
		if !hasPreview {
			return "", errors.New("preview release has no declared preview actions or fields")
		}
		return previewChannel, nil
	}
	if hasPreview {
		return "", errors.New("stable release cannot contain preview actions or fields")
	}
	return stableChannel, nil
}

func runCurrentChannel(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("current-channel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", ".", "agr-go-sdk repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("current-channel accepts no positional arguments")
	}
	routing, err := readRouting(*root)
	if err != nil {
		return err
	}
	channel := stableChannel
	if hasPreviewCapabilities(routing) {
		channel = previewChannel
	}
	fmt.Fprintln(stdout, channel)
	return nil
}

func hasPreviewCapabilities(routing routingContract) bool {
	if len(routing.PreviewFields) > 0 {
		return true
	}
	for _, action := range routing.Actions {
		if action.Maturity == previewChannel {
			return true
		}
	}
	return false
}

func selectChannels(requested string, configured []string) ([]string, error) {
	available := map[string]bool{}
	for _, channel := range configured {
		available[channel] = true
	}
	switch requested {
	case allChannels:
		return append([]string(nil), configured...), nil
	case stableChannel, previewChannel:
		if !available[requested] {
			return nil, fmt.Errorf("contract channel %q is not configured", requested)
		}
		return []string{requested}, nil
	default:
		return nil, fmt.Errorf("unsupported contract channel %q", requested)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func contractPath(routing routingContract, channel, name string) string {
	return filepath.Join("contracts", "controlplane", routing.Service, "v"+routing.Version, channel, name)
}

func routingForChannel(routing routingContract, channel string) routingContract {
	selected := routing
	selected.Channels = []string{channel}
	selected.Actions = nil
	selected.CommonActions = nil
	selected.PreviewFields = []fieldMaturity{}
	for _, action := range routing.Actions {
		if channel == stableChannel && action.Maturity == previewChannel {
			continue
		}
		selected.Actions = append(selected.Actions, action)
		if action.Route == string(controlplane.CommonClient) {
			selected.CommonActions = append(selected.CommonActions, action.Name)
		}
	}
	if channel == previewChannel {
		selected.PreviewFields = append([]fieldMaturity{}, routing.PreviewFields...)
	}
	return selected
}

func readRouting(root string) (routingContract, error) {
	data, err := os.ReadFile(filepath.Join(root, routingFile))
	if err != nil {
		return routingContract{}, fmt.Errorf("read action routing: %w", err)
	}
	var routing routingContract
	if err := json.Unmarshal(data, &routing); err != nil {
		return routingContract{}, fmt.Errorf("decode action routing: %w", err)
	}
	if routing.Service == "" || routing.Version == "" || len(routing.Actions) == 0 || !reflect.DeepEqual(routing.Channels, []string{stableChannel, previewChannel}) {
		return routingContract{}, errors.New("action routing metadata is incomplete")
	}
	seen, common := map[string]bool{}, map[string]bool{}
	for _, name := range routing.CommonActions {
		if common[name] || name == "" {
			return routingContract{}, fmt.Errorf("duplicate or empty common action %q", name)
		}
		common[name] = true
	}
	previous := ""
	for _, action := range routing.Actions {
		if action.Name == "" || action.Resource == "" || seen[action.Name] {
			return routingContract{}, fmt.Errorf("duplicate or empty action %q", action.Name)
		}
		if previous != "" && action.Name < previous {
			return routingContract{}, errors.New("action routing must be sorted by action name")
		}
		if action.Route != string(controlplane.OfficialTyped) && action.Route != string(controlplane.CommonClient) {
			return routingContract{}, fmt.Errorf("action %s has unsupported route %q", action.Name, action.Route)
		}
		if action.Maturity != stableChannel && action.Maturity != previewChannel {
			return routingContract{}, fmt.Errorf("action %s has unsupported maturity %q", action.Name, action.Maturity)
		}
		if common[action.Name] != (action.Route == string(controlplane.CommonClient)) {
			return routingContract{}, fmt.Errorf("action %s route and common_actions disagree", action.Name)
		}
		seen[action.Name], previous = true, action.Name
	}
	for name := range common {
		if !seen[name] {
			return routingContract{}, fmt.Errorf("common action %s is not in the action registry", name)
		}
	}
	seenFields := map[string]bool{}
	for _, field := range routing.PreviewFields {
		if field.Object == "" || field.Field == "" || field.Maturity != previewChannel {
			return routingContract{}, errors.New("preview field metadata is incomplete")
		}
		key := field.Object + "." + field.Field
		if seenFields[key] {
			return routingContract{}, fmt.Errorf("duplicate preview field %s", key)
		}
		seenFields[key] = true
	}
	return routing, nil
}

func manifestActions(api apiContract, routing routingContract) ([]manifestAction, error) {
	out := make([]manifestAction, 0, len(routing.Actions))
	for _, route := range routing.Actions {
		raw, ok := api.Actions[route.Name]
		if !ok {
			return nil, fmt.Errorf("manifest action %s is absent", route.Name)
		}
		var shape actionShape
		if err := json.Unmarshal(raw, &shape); err != nil {
			return nil, fmt.Errorf("decode manifest action %s: %w", route.Name, err)
		}
		closure, err := reachableObjects(map[string]json.RawMessage{route.Name: raw}, api.Objects)
		if err != nil {
			return nil, err
		}
		objects := make([]string, 0, len(closure))
		for name := range closure {
			objects = append(objects, name)
		}
		sort.Strings(objects)
		out = append(out, manifestAction{Name: route.Name, Resource: route.Resource, Maturity: route.Maturity, Route: route.Route, Request: shape.Input, Response: shape.Output, Objects: objects})
	}
	return out, nil
}

func pruneContract(full apiContract, routing routingContract) (apiContract, error) {
	metadata, err := sdkMetadata(full.Metadata)
	if err != nil {
		return apiContract{}, err
	}
	reduced := apiContract{Actions: map[string]json.RawMessage{}, Metadata: metadata, Objects: map[string]json.RawMessage{}, Version: full.Version}
	for _, route := range routing.Actions {
		raw, ok := full.Actions[route.Name]
		if !ok {
			return apiContract{}, fmt.Errorf("allowlisted action %s is absent from CLI effective API", route.Name)
		}
		reduced.Actions[route.Name] = raw
	}
	needed, err := reachableObjects(reduced.Actions, full.Objects)
	if err != nil {
		return apiContract{}, err
	}
	for name := range needed {
		reduced.Objects[name] = full.Objects[name]
	}
	if _, err := validateContract(reduced, routing); err != nil {
		return apiContract{}, err
	}
	return reduced, nil
}

// sdkMetadata keeps source identity fields while replacing the full-service brief, which also
// describes API-key Actions outside this SDK's reviewed allowlist.
func sdkMetadata(raw json.RawMessage) (json.RawMessage, error) {
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode CLI metadata: %w", err)
	}
	if _, exists := metadata["api_brief"]; exists {
		metadata["api_brief"] = sdkAPIBrief
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode SDK metadata: %w", err)
	}
	return data, nil
}

func validateContract(api apiContract, routing routingContract) (map[string]bool, error) {
	if len(api.Actions) != len(routing.Actions) {
		return nil, fmt.Errorf("effective snapshot has %d actions, routing declares %d", len(api.Actions), len(routing.Actions))
	}
	for _, route := range routing.Actions {
		if _, ok := api.Actions[route.Name]; !ok {
			return nil, fmt.Errorf("routed action %s is absent from effective snapshot", route.Name)
		}
	}
	closure, err := reachableObjects(api.Actions, api.Objects)
	if err != nil {
		return nil, err
	}
	if len(closure) != len(api.Objects) {
		extra := make([]string, 0)
		for name := range api.Objects {
			if !closure[name] {
				extra = append(extra, name)
			}
		}
		sort.Strings(extra)
		return nil, fmt.Errorf("effective snapshot contains objects outside routed action closure: %s", strings.Join(extra, ", "))
	}
	for name, raw := range api.Objects {
		var object objectShape
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, fmt.Errorf("decode object %s: %w", name, err)
		}
		members := map[string]bool{}
		for index, member := range object.Members {
			if member.Name == "" || member.Type == "" || member.Member == "" {
				return nil, fmt.Errorf("object %s member %d lacks name, type, or member", name, index)
			}
			if members[member.Name] {
				return nil, fmt.Errorf("object %s has duplicate member %s", name, member.Name)
			}
			members[member.Name] = true
		}
	}
	return closure, nil
}

func validatePreviewSuperset(stable, preview apiContract, fields []fieldMaturity) error {
	if stable.Version != preview.Version || !jsonEqual(stable.Metadata, preview.Metadata) {
		return errors.New("preview contract changed stable version or metadata")
	}
	declared := map[string]bool{}
	for _, field := range fields {
		key := field.Object + "." + field.Field
		if declared[key] {
			return fmt.Errorf("duplicate preview field declaration %s", key)
		}
		declared[key] = true
	}
	used := map[string]bool{}
	for name, stableAction := range stable.Actions {
		previewAction, ok := preview.Actions[name]
		if !ok {
			return fmt.Errorf("preview contract removed stable action %s", name)
		}
		if !jsonEqual(stableAction, previewAction) {
			return fmt.Errorf("preview contract changed stable action %s", name)
		}
	}
	for name, stableObject := range stable.Objects {
		previewObject, ok := preview.Objects[name]
		if !ok {
			return fmt.Errorf("preview contract removed stable object %s", name)
		}
		if err := comparePreviewObject(name, stableObject, previewObject, declared, used); err != nil {
			return err
		}
	}
	for key := range declared {
		if !used[key] {
			return fmt.Errorf("declared preview field %s is not an added field on a stable object", key)
		}
	}
	return nil
}

// filterPreviewFields removes patched fields from objects used by stable SDK Actions unless the
// field is explicitly selected in preview_fields. Preview-only Actions and their private object
// graphs remain governed by the Action allowlist.
func filterPreviewFields(stable apiContract, preview *apiContract, fields []fieldMaturity) error {
	declared := map[string]bool{}
	for _, field := range fields {
		key := field.Object + "." + field.Field
		if declared[key] {
			return fmt.Errorf("duplicate preview field declaration %s", key)
		}
		declared[key] = true
	}
	used := map[string]bool{}
	for name, stableRaw := range stable.Objects {
		previewRaw, ok := preview.Objects[name]
		if !ok {
			return fmt.Errorf("preview source removed stable object %s", name)
		}
		filtered, selected, err := filteredPreviewObject(name, stableRaw, previewRaw, declared)
		if err != nil {
			return err
		}
		for key := range selected {
			used[key] = true
		}
		preview.Objects[name] = filtered
	}
	for key := range declared {
		if !used[key] {
			return fmt.Errorf("declared preview field %s is absent from the CLI preview delta", key)
		}
	}
	return nil
}

func filteredPreviewObject(name string, stableRaw, previewRaw json.RawMessage, declared map[string]bool) (json.RawMessage, map[string]bool, error) {
	var stableObject, previewObject map[string]json.RawMessage
	if err := json.Unmarshal(stableRaw, &stableObject); err != nil {
		return nil, nil, fmt.Errorf("decode stable object %s: %w", name, err)
	}
	if err := json.Unmarshal(previewRaw, &previewObject); err != nil {
		return nil, nil, fmt.Errorf("decode preview object %s: %w", name, err)
	}
	for key, stableValue := range stableObject {
		if key == "members" {
			continue
		}
		previewValue, ok := previewObject[key]
		if !ok || !jsonEqual(stableValue, previewValue) {
			return nil, nil, fmt.Errorf("CLI preview changed stable object %s property %s", name, key)
		}
	}
	for key := range previewObject {
		if key != "members" {
			if _, ok := stableObject[key]; !ok {
				return nil, nil, fmt.Errorf("CLI preview added unsupported object property %s.%s", name, key)
			}
		}
	}
	stableMembers, err := objectMembers(stableObject["members"])
	if err != nil {
		return nil, nil, fmt.Errorf("decode stable object %s members: %w", name, err)
	}
	var previewMembers []json.RawMessage
	if err := json.Unmarshal(previewObject["members"], &previewMembers); err != nil {
		return nil, nil, fmt.Errorf("decode preview object %s members: %w", name, err)
	}
	filtered := make([]json.RawMessage, 0, len(previewMembers))
	selected := map[string]bool{}
	seenStable := map[string]bool{}
	for _, memberRaw := range previewMembers {
		var member struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(memberRaw, &member); err != nil {
			return nil, nil, fmt.Errorf("decode preview field on %s: %w", name, err)
		}
		if stableMember, ok := stableMembers[member.Name]; ok {
			if !jsonEqual(stableMember, memberRaw) {
				return nil, nil, fmt.Errorf("CLI preview changed stable field %s.%s", name, member.Name)
			}
			seenStable[member.Name] = true
			filtered = append(filtered, memberRaw)
			continue
		}
		key := name + "." + member.Name
		if declared[key] {
			selected[key] = true
			filtered = append(filtered, memberRaw)
		}
	}
	for member := range stableMembers {
		if !seenStable[member] {
			return nil, nil, fmt.Errorf("CLI preview removed stable field %s.%s", name, member)
		}
	}
	encodedMembers, err := json.Marshal(filtered)
	if err != nil {
		return nil, nil, fmt.Errorf("encode preview object %s members: %w", name, err)
	}
	previewObject["members"] = encodedMembers
	encodedObject, err := json.Marshal(previewObject)
	if err != nil {
		return nil, nil, fmt.Errorf("encode preview object %s: %w", name, err)
	}
	return encodedObject, selected, nil
}

func comparePreviewObject(name string, stableRaw, previewRaw json.RawMessage, declared, used map[string]bool) error {
	var stableObject, previewObject map[string]json.RawMessage
	if err := json.Unmarshal(stableRaw, &stableObject); err != nil {
		return fmt.Errorf("decode stable object %s: %w", name, err)
	}
	if err := json.Unmarshal(previewRaw, &previewObject); err != nil {
		return fmt.Errorf("decode preview object %s: %w", name, err)
	}
	for key, stableValue := range stableObject {
		if key == "members" {
			continue
		}
		previewValue, ok := previewObject[key]
		if !ok || !jsonEqual(stableValue, previewValue) {
			return fmt.Errorf("preview contract changed stable object %s property %s", name, key)
		}
	}
	for key := range previewObject {
		if key != "members" {
			if _, ok := stableObject[key]; !ok {
				return fmt.Errorf("preview contract added undeclared object property %s.%s", name, key)
			}
		}
	}
	stableMembers, err := objectMembers(stableObject["members"])
	if err != nil {
		return fmt.Errorf("decode stable object %s members: %w", name, err)
	}
	previewMembers, err := objectMembers(previewObject["members"])
	if err != nil {
		return fmt.Errorf("decode preview object %s members: %w", name, err)
	}
	for member, stableValue := range stableMembers {
		previewValue, ok := previewMembers[member]
		if !ok {
			return fmt.Errorf("preview contract removed stable field %s.%s", name, member)
		}
		if !jsonEqual(stableValue, previewValue) {
			return fmt.Errorf("preview contract changed stable field %s.%s", name, member)
		}
	}
	for member := range previewMembers {
		if _, ok := stableMembers[member]; ok {
			continue
		}
		key := name + "." + member
		if !declared[key] {
			return fmt.Errorf("preview contract added undeclared field %s", key)
		}
		used[key] = true
	}
	return nil
}

func objectMembers(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, errors.New("members are missing")
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(values))
	for _, value := range values {
		var member struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(value, &member); err != nil {
			return nil, err
		}
		if member.Name == "" || out[member.Name] != nil {
			return nil, fmt.Errorf("duplicate or empty member %q", member.Name)
		}
		out[member.Name] = value
	}
	return out, nil
}

func jsonEqual(first, second json.RawMessage) bool {
	var left, right any
	leftDecoder := json.NewDecoder(bytes.NewReader(first))
	leftDecoder.UseNumber()
	rightDecoder := json.NewDecoder(bytes.NewReader(second))
	rightDecoder.UseNumber()
	return leftDecoder.Decode(&left) == nil && rightDecoder.Decode(&right) == nil && reflect.DeepEqual(left, right)
}

func reachableObjects(actions, objects map[string]json.RawMessage) (map[string]bool, error) {
	needed := map[string]bool{}
	pending := make([]string, 0)
	for name, raw := range actions {
		var action actionShape
		if err := json.Unmarshal(raw, &action); err != nil {
			return nil, fmt.Errorf("decode action %s: %w", name, err)
		}
		if action.Input == "" || action.Output == "" {
			return nil, fmt.Errorf("action %s lacks input or output shape", name)
		}
		for _, shape := range []string{action.Input, action.Output} {
			if _, ok := objects[shape]; !ok {
				return nil, fmt.Errorf("action %s references missing object %s", name, shape)
			}
			if !needed[shape] {
				needed[shape] = true
				pending = append(pending, shape)
			}
		}
	}
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		var object objectShape
		if err := json.Unmarshal(objects[name], &object); err != nil {
			return nil, fmt.Errorf("decode object %s: %w", name, err)
		}
		for _, member := range object.Members {
			if _, ok := objects[member.Member]; ok && !needed[member.Member] {
				needed[member.Member] = true
				pending = append(pending, member.Member)
			}
		}
	}
	return needed, nil
}

func compareImplementationRoutes(routing routingContract) error {
	want := make(map[string]controlplane.ActionRoute, len(routing.Actions))
	for _, action := range routing.Actions {
		want[action.Name] = controlplane.ActionRoute(action.Route)
	}
	got := controlplane.ActionRoutes()
	if len(got) != len(want) {
		return fmt.Errorf("implementation has %d action routes, contract declares %d", len(got), len(want))
	}
	for name, route := range want {
		if got[name] != route {
			return fmt.Errorf("implementation route for %s is %q, expected %q", name, got[name], route)
		}
	}
	return nil
}

func routeCounts(routing routingContract) (typed, common int) {
	for _, action := range routing.Actions {
		if action.Route == string(controlplane.OfficialTyped) {
			typed++
		} else {
			common++
		}
	}
	return typed, common
}

func commandOutput(directory, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = directory
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func compactJSON(data []byte) ([]byte, error) {
	var output bytes.Buffer
	if err := json.Compact(&output, data); err != nil {
		return nil, err
	}
	output.WriteByte('\n')
	return output.Bytes(), nil
}

func marshalFile(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode generated contract: %w", err)
	}
	return append(data, '\n'), nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create contract directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".controlcontract-*")
	if err != nil {
		return fmt.Errorf("create temporary contract: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary contract: %w", err)
	}
	if err = temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set contract permissions: %w", err)
	}
	if err = temporary.Close(); err != nil {
		return fmt.Errorf("close temporary contract: %w", err)
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace generated contract: %w", err)
	}
	return nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
