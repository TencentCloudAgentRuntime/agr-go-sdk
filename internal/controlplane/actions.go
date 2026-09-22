package controlplane

// ActionRoute is the compile-time transport choice for an approved public Cloud action.
type ActionRoute string

const (
	// OfficialTyped binds an action directly to the generated Tencent Cloud client method.
	OfficialTyped ActionRoute = "official_typed"
	// CommonRaw binds an allowlisted effective-API action that the official SDK has not generated.
	// It is never a failure fallback.
	CommonRaw ActionRoute = "common_raw"
)

var actionRoutes = map[string]ActionRoute{
	"AcquireDeploymentToken":            OfficialTyped,
	"StartSandboxInstance":              OfficialTyped,
	"DescribeSandboxInstanceList":       OfficialTyped,
	"AcquireSandboxInstanceToken":       OfficialTyped,
	"AppendEvent":                       CommonRaw,
	"ApproveRegistryRecord":             CommonRaw,
	"CancelRegistryRecord":              CommonRaw,
	"CreatePreCacheImageTask":           OfficialTyped,
	"CreateRegistry":                    CommonRaw,
	"CreateRegistryRecord":              CommonRaw,
	"CreateDeployment":                  OfficialTyped,
	"CreateSandboxTool":                 OfficialTyped,
	"CreateSession":                     CommonRaw,
	"CreateSessionSpace":                CommonRaw,
	"CreateVolume":                      CommonRaw,
	"CreateVolumeTemplate":              CommonRaw,
	"DeleteSandboxTool":                 OfficialTyped,
	"DeleteDeployment":                  OfficialTyped,
	"DeleteRegistry":                    CommonRaw,
	"DeleteRegistryRecord":              CommonRaw,
	"DeleteSession":                     CommonRaw,
	"DeleteSessionSpace":                CommonRaw,
	"DeleteVolume":                      CommonRaw,
	"DeleteVolumeTemplate":              CommonRaw,
	"DescribeDeployment":                OfficialTyped,
	"DescribeDeploymentList":            OfficialTyped,
	"DescribeEvents":                    CommonRaw,
	"DescribePreCacheImageTask":         OfficialTyped,
	"DescribeRegistry":                  CommonRaw,
	"DescribeRegistryAuditLogList":      CommonRaw,
	"DescribeRegistryList":              CommonRaw,
	"DescribeRegistryRecord":            CommonRaw,
	"DescribeRegistryRecordList":        CommonRaw,
	"DescribeRegistryRecordVersionList": CommonRaw,
	"DescribeSandboxToolList":           OfficialTyped,
	"DescribeSession":                   CommonRaw,
	"DescribeSessionSpace":              CommonRaw,
	"DescribeSessionSpaces":             CommonRaw,
	"DescribeSessions":                  CommonRaw,
	"DescribeVolumeList":                CommonRaw,
	"DescribeVolumeTemplateList":        CommonRaw,
	"PauseSandboxInstance":              OfficialTyped,
	"PreviewRegistryRecord":             CommonRaw,
	"RejectRegistryRecord":              CommonRaw,
	"ModifyDeployment":                  OfficialTyped,
	"ModifySession":                     CommonRaw,
	"ModifySessionSpace":                CommonRaw,
	"ResumeSandboxInstance":             OfficialTyped,
	"StopSandboxInstance":               OfficialTyped,
	"SyncRegistryRecord":                CommonRaw,
	"UpdateRegistry":                    CommonRaw,
	"UpdateRegistryRecord":              CommonRaw,
	"UpdateSandboxInstance":             OfficialTyped,
	"UpdateSandboxTool":                 OfficialTyped,
	"UpdateVolume":                      CommonRaw,
	"UpdateVolumeTemplate":              CommonRaw,
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
