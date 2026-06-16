// Package compute is the public SDK surface for Shed sandbox compute plugins.
package compute

import (
	internal "github.com/brianmichel/shed/internal/compute"
)

const (
	APIVersionV1     = internal.APIVersionV1
	PluginMapKey     = internal.PluginMapKey
	ProtocolVersion  = internal.ProtocolVersion
	MagicCookieKey   = internal.MagicCookieKey
	MagicCookieValue = internal.MagicCookieValue
)

type PluginInfo = internal.PluginInfo
type AllocateRequest = internal.AllocateRequest
type AllocateResponse = internal.AllocateResponse
type StatusRequest = internal.StatusRequest
type StatusResponse = internal.StatusResponse
type RenewRequest = internal.RenewRequest
type RenewResponse = internal.RenewResponse
type ReleaseRequest = internal.ReleaseRequest
type ReleaseResponse = internal.ReleaseResponse
type ExecRequest = internal.ExecRequest
type ExecEvent = internal.ExecEvent
type ExecStdinRequest = internal.ExecStdinRequest
type ExecSignalRequest = internal.ExecSignalRequest
type ExecControlResponse = internal.ExecControlResponse
type ExecEventSink = internal.ExecEventSink
type ComputeV1 = internal.ComputeV1

func ServePlugin(impl ComputeV1) { internal.ServePlugin(impl) }
func HasAPIVersion(info PluginInfo, apiVersion string) bool {
	return internal.HasAPIVersion(info, apiVersion)
}
