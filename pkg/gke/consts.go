package gke

// Status indicates how to handle the response from a request to update a resource
type Status int

// Status indicators
const (
	// Changed means the request to change resource was accepted and change is in progress
	Changed Status = iota
	// Retry means the request to change resource was rejected due to an expected error and should be retried later
	Retry
	// NotChanged means the resource was not changed, either due to error or because it was unnecessary
	NotChanged
)

// Error strings from the provider
const (
	errNotFound = "notFound"
	errWait     = "Please wait and try again once it is done"
)

// Release channels
const (
	// ReleaseChannelUnspecified is the release channel used by clusters that are
	// not enrolled in any GKE release channel, displayed as "No channel" in GCP.
	// GKE no longer accepts this value when creating a cluster, so clusters that
	// should not be enrolled in a channel are created in a temporary channel and
	// unenrolled once they are running.
	ReleaseChannelUnspecified = "UNSPECIFIED"
	// ReleaseChannelRapid is the RAPID release channel.
	ReleaseChannelRapid = "RAPID"
	// ReleaseChannelRegular is the REGULAR release channel.
	ReleaseChannelRegular = "REGULAR"
	// ReleaseChannelStable is the STABLE release channel.
	ReleaseChannelStable = "STABLE"
	// ReleaseChannelExtended is the EXTENDED release channel.
	ReleaseChannelExtended = "EXTENDED"
	// ReleaseChannelNone is the user facing alias for ReleaseChannelUnspecified.
	ReleaseChannelNone = "NONE"
)

// releaseChannelPreference is the order in which a temporary release channel is
// picked when a cluster is created without being enrolled in a channel. It
// follows the order GKE itself uses to pick a channel for a given version, from
// the most mature channel to the least, with EXTENDED last because it is the
// only channel that keeps older minor versions available.
var releaseChannelPreference = []string{
	ReleaseChannelStable,
	ReleaseChannelRegular,
	ReleaseChannelRapid,
	ReleaseChannelExtended,
}
