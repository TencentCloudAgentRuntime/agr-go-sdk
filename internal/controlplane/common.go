package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	tccommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tchttp "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/http"
)

const agsAPIVersion = "2025-09-20"

// commonCall invokes only Actions whose reviewed route is CommonClient. It is not
// a fallback for typed requests and is deliberately unavailable outside this
// package.
func (a *AGS) commonCall(ctx context.Context, action string, input, output any) error {
	if actionRoutes[action] != CommonClient {
		return &Error{Code: "ClientError.ActionNotAllowed", Cause: errors.New("action is not approved for CommonClient routing")}
	}
	return a.sendCommon(ctx, action, input, output)
}

// commonPreviewFieldCall covers a stable typed Action whose generated model
// does not yet contain an explicitly reviewed Preview field. The separate
// field-level allowlist prevents this compatibility path from becoming an
// arbitrary Action API.
func (a *AGS) commonPreviewFieldCall(ctx context.Context, action string, input, output any) error {
	switch action {
	case "CreateSandboxTool", "DescribeSandboxToolList":
		return a.sendCommon(ctx, action, input, output)
	default:
		return &Error{Code: "ClientError.ActionNotAllowed", Cause: errors.New("action is not approved for Preview field routing")}
	}
}

func (a *AGS) sendCommon(ctx context.Context, action string, input, output any) error {
	credential, err := a.credential(ctx)
	if err != nil {
		return err
	}
	if credential.SecretID == "" || credential.SecretKey == "" {
		return &Error{Code: "AuthFailure.CredentialMissing"}
	}
	clientProfile, err := clientProfile(a.config)
	if err != nil {
		return err
	}
	signed := tccommon.NewCredential(credential.SecretID, credential.SecretKey)
	if credential.Token != "" {
		signed = tccommon.NewTokenCredential(credential.SecretID, credential.SecretKey, credential.Token)
	}
	client := tccommon.NewCommonClient(signed, a.config.Region, clientProfile)
	client.WithHttpTransport(noRedirect(a.config.Transport))

	payload, err := json.Marshal(input)
	if err != nil {
		return &Error{Code: "ClientError.InvalidRequest", Cause: errors.New("encode reviewed CommonClient request")}
	}
	request := tchttp.NewCommonRequest("ags", agsAPIVersion, action)
	if err := request.SetActionParameters(payload); err != nil {
		return wrap(err)
	}
	callCtx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()
	request.SetContext(callCtx)
	response := tchttp.NewCommonResponse()
	if err := client.Send(request, response); err != nil {
		return wrapCall(callCtx, err)
	}
	var envelope struct {
		Response json.RawMessage `json:"Response"`
	}
	body := response.GetBody()
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Response) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Response), []byte("null")) {
		return &Error{Code: "ClientError.MalformedResponse", Cause: errors.New("CommonClient response envelope is malformed")}
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Response, output); err != nil {
		return &Error{Code: "ClientError.MalformedResponse", Cause: errors.New("CommonClient response payload is malformed")}
	}
	return nil
}
