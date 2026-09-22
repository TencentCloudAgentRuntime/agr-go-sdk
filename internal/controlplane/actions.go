package controlplane

// ActionRoute is the compile-time transport choice for an approved public Cloud action.
type ActionRoute string

const (
	// OfficialTyped binds an action directly to the generated Tencent Cloud client method.
	OfficialTyped ActionRoute = "official_typed"
	// CommonClient binds an allowlisted effective-API action that the official SDK has not generated.
	// It is never a failure fallback.
	CommonClient ActionRoute = "common_client"
)

var actionRoutes = map[string]ActionRoute{
	"AcquireDeploymentToken":            OfficialTyped,
	"StartSandboxInstance":              OfficialTyped,
	"DescribeSandboxInstanceList":       OfficialTyped,
	"AcquireSandboxInstanceToken":       OfficialTyped,
	"AppendEvent":                       CommonClient,
	"ApproveRegistryRecord":             CommonClient,
	"CancelRegistryRecord":              CommonClient,
	"CreatePreCacheImageTask":           OfficialTyped,
	"CreateRegistry":                    CommonClient,
	"CreateRegistryRecord":              CommonClient,
	"CreateDeployment":                  OfficialTyped,
	"CreateSandboxTool":                 OfficialTyped,
	"CreateSession":                     CommonClient,
	"CreateSessionSpace":                CommonClient,
	"CreateVolume":                      CommonClient,
	"CreateVolumeTemplate":              CommonClient,
	"DeleteSandboxTool":                 OfficialTyped,
	"DeleteDeployment":                  OfficialTyped,
	"DeleteRegistry":                    CommonClient,
	"DeleteRegistryRecord":              CommonClient,
	"DeleteSession":                     CommonClient,
	"DeleteSessionSpace":                CommonClient,
	"DeleteVolume":                      CommonClient,
	"DeleteVolumeTemplate":              CommonClient,
	"DescribeDeployment":                OfficialTyped,
	"DescribeDeploymentList":            OfficialTyped,
	"DescribeEvents":                    CommonClient,
	"DescribePreCacheImageTask":         OfficialTyped,
	"DescribeRegistry":                  CommonClient,
	"DescribeRegistryAuditLogList":      CommonClient,
	"DescribeRegistryList":              CommonClient,
	"DescribeRegistryRecord":            CommonClient,
	"DescribeRegistryRecordList":        CommonClient,
	"DescribeRegistryRecordVersionList": CommonClient,
	"DescribeSandboxToolList":           OfficialTyped,
	"DescribeSession":                   CommonClient,
	"DescribeSessionSpace":              CommonClient,
	"DescribeSessionSpaces":             CommonClient,
	"DescribeSessions":                  CommonClient,
	"DescribeVolumeList":                CommonClient,
	"DescribeVolumeTemplateList":        CommonClient,
	"GetSkillPackageDownloadURL":        CommonClient,
	"GetSkillPackageUploadURL":          CommonClient,
	"PauseSandboxInstance":              OfficialTyped,
	"PreviewRegistryRecord":             CommonClient,
	"RejectRegistryRecord":              CommonClient,
	"ModifyDeployment":                  OfficialTyped,
	"ModifySession":                     CommonClient,
	"ModifySessionSpace":                CommonClient,
	"ResumeSandboxInstance":             OfficialTyped,
	"StopSandboxInstance":               OfficialTyped,
	"SyncRegistryRecord":                CommonClient,
	"UpdateRegistry":                    CommonClient,
	"UpdateRegistryRecord":              CommonClient,
	"UpdateSandboxInstance":             OfficialTyped,
	"UpdateSandboxTool":                 OfficialTyped,
	"UpdateVolume":                      CommonClient,
	"UpdateVolumeTemplate":              CommonClient,
}

// ActionRoutes returns a copy of the approved static action registry. The package exposes no
// arbitrary action invocation and never changes route after a request failure.
func ActionRoutes() map[string]ActionRoute {
	out := make(map[string]ActionRoute, len(actionRoutes))
	for action, route := range actionRoutes {
		out[action] = route
	}
	return out
}
