package gke

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	gkeapi "google.golang.org/api/container/v1"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services"
	"github.com/rancher/gke-operator/pkg/utils"
)

// Errors
const (
	cannotBeNilError            = "field [%s] cannot be nil for non-import cluster [%s (id: %s)]"
	cannotBeNilForNodePoolError = "field [%s] cannot be nil for nodepool [%s] in non-nil cluster [%s (id: %s)]"
)

// Create creates an upstream GKE cluster.
func Create(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) error {
	err := validateCreateRequest(ctx, gkeClient, config)
	if err != nil {
		return err
	}

	createChannel, err := releaseChannelForCreate(ctx, gkeClient, config)
	if err != nil {
		return err
	}

	// The create request is built from a copy of the config so that the release
	// channel resolved above is the one used to build the request, without
	// mutating the config owned by the caller.
	createConfig := config.DeepCopy()
	createConfig.Spec.ReleaseChannel = &createChannel

	createClusterRequest := NewClusterCreateRequest(createConfig)

	_, err = gkeClient.ClusterCreate(ctx,
		LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
		createClusterRequest)

	return err
}

// CreateNodePool creates an upstream node pool with the given cluster as a parent.
func CreateNodePool(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig, nodePoolConfig *gkev1.GKENodePoolConfig) (Status, error) {
	err := validateNodePoolCreateRequest(nodePoolConfig, config)
	if err != nil {
		return NotChanged, err
	}

	createNodePoolRequest, err := newNodePoolCreateRequest(
		nodePoolConfig,
		config,
	)
	if err != nil {
		return NotChanged, err
	}

	_, err = gkeClient.NodePoolCreate(ctx,
		ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
		createNodePoolRequest)
	if err != nil && strings.Contains(err.Error(), errWait) {
		return Retry, nil
	}
	if err != nil {
		return NotChanged, err
	}

	return Changed, nil
}

// NewClusterCreateRequest creates a CreateClusterRequest that can be submitted to GKE
func NewClusterCreateRequest(config *gkev1.GKEClusterConfig) *gkeapi.CreateClusterRequest {
	enableKubernetesAlpha := config.Spec.EnableKubernetesAlpha != nil && *config.Spec.EnableKubernetesAlpha
	request := &gkeapi.CreateClusterRequest{
		Cluster: &gkeapi.Cluster{
			Name:                  config.Spec.ClusterName,
			Description:           config.Spec.Description,
			ResourceLabels:        config.Spec.Labels,
			InitialClusterVersion: *config.Spec.KubernetesVersion,
			EnableKubernetesAlpha: enableKubernetesAlpha,
			ClusterIpv4Cidr:       *config.Spec.ClusterIpv4CidrBlock,
			LoggingService:        *config.Spec.LoggingService,
			MonitoringService:     *config.Spec.MonitoringService,
			IpAllocationPolicy: &gkeapi.IPAllocationPolicy{
				ClusterIpv4CidrBlock:       config.Spec.IPAllocationPolicy.ClusterIpv4CidrBlock,
				ClusterSecondaryRangeName:  config.Spec.IPAllocationPolicy.ClusterSecondaryRangeName,
				CreateSubnetwork:           config.Spec.IPAllocationPolicy.CreateSubnetwork,
				NodeIpv4CidrBlock:          config.Spec.IPAllocationPolicy.NodeIpv4CidrBlock,
				ServicesIpv4CidrBlock:      config.Spec.IPAllocationPolicy.ServicesIpv4CidrBlock,
				ServicesSecondaryRangeName: config.Spec.IPAllocationPolicy.ServicesSecondaryRangeName,
				SubnetworkName:             config.Spec.IPAllocationPolicy.SubnetworkName,
				UseIpAliases:               config.Spec.IPAllocationPolicy.UseIPAliases,
			},
			AddonsConfig:      &gkeapi.AddonsConfig{},
			NodePools:         []*gkeapi.NodePool{},
			Locations:         config.Spec.Locations,
			MaintenancePolicy: &gkeapi.MaintenancePolicy{},
		},
	}

	if *config.Spec.MaintenanceWindow != "" {
		request.Cluster.MaintenancePolicy.Window = &gkeapi.MaintenanceWindow{
			DailyMaintenanceWindow: &gkeapi.DailyMaintenanceWindow{
				StartTime: *config.Spec.MaintenanceWindow,
			},
		}
	}

	if config.Spec.AutopilotConfig != nil && config.Spec.AutopilotConfig.Enabled {
		request.Cluster.Autopilot = &gkeapi.Autopilot{
			Enabled: config.Spec.AutopilotConfig.Enabled,
		}
	} else {
		addons := config.Spec.ClusterAddons
		request.Cluster.AddonsConfig.HttpLoadBalancing = &gkeapi.HttpLoadBalancing{Disabled: !addons.HTTPLoadBalancing}
		request.Cluster.AddonsConfig.HorizontalPodAutoscaling = &gkeapi.HorizontalPodAutoscaling{Disabled: !addons.HorizontalPodAutoscaling}
		request.Cluster.AddonsConfig.NetworkPolicyConfig = &gkeapi.NetworkPolicyConfig{Disabled: !addons.NetworkPolicyConfig}

		request.Cluster.NodePools = make([]*gkeapi.NodePool, 0, len(config.Spec.NodePools))

		for np := range config.Spec.NodePools {
			nodePool := newGKENodePoolFromConfig(&config.Spec.NodePools[np], config)
			request.Cluster.NodePools = append(request.Cluster.NodePools, nodePool)
		}

		if config.Spec.MasterAuthorizedNetworksConfig != nil {
			blocks := make([]*gkeapi.CidrBlock, 0, len(config.Spec.MasterAuthorizedNetworksConfig.CidrBlocks))
			for _, b := range config.Spec.MasterAuthorizedNetworksConfig.CidrBlocks {
				blocks = append(blocks, &gkeapi.CidrBlock{
					CidrBlock:   b.CidrBlock,
					DisplayName: b.DisplayName,
				})
			}
			request.Cluster.MasterAuthorizedNetworksConfig = &gkeapi.MasterAuthorizedNetworksConfig{
				Enabled:    config.Spec.MasterAuthorizedNetworksConfig.Enabled,
				CidrBlocks: blocks,
			}
		}
	}

	if config.Spec.Network != nil {
		request.Cluster.Network = *config.Spec.Network
	}
	if config.Spec.Subnetwork != nil {
		request.Cluster.Subnetwork = *config.Spec.Subnetwork
	}

	if config.Spec.NetworkPolicyEnabled != nil {
		request.Cluster.NetworkPolicy = &gkeapi.NetworkPolicy{
			Enabled: *config.Spec.NetworkPolicyEnabled,
		}
	}

	if config.Spec.PrivateClusterConfig != nil && config.Spec.PrivateClusterConfig.EnablePrivateNodes {
		request.Cluster.PrivateClusterConfig = &gkeapi.PrivateClusterConfig{
			EnablePrivateEndpoint: config.Spec.PrivateClusterConfig.EnablePrivateEndpoint,
			EnablePrivateNodes:    config.Spec.PrivateClusterConfig.EnablePrivateNodes,
			MasterIpv4CidrBlock:   config.Spec.PrivateClusterConfig.MasterIpv4CidrBlock,
		}
	}

	request.Cluster.ReleaseChannel = newClusterReleaseChannel(config)

	// GKE does not allow node auto-upgrade to be disabled while a cluster is
	// enrolled in a release channel. Clusters that are only enrolled so that they
	// can be created are unenrolled once they are running, and the reconcile loop
	// then restores the configured node management settings.
	if request.Cluster.ReleaseChannel != nil {
		for _, np := range request.Cluster.NodePools {
			if np.Management == nil {
				np.Management = &gkeapi.NodeManagement{}
			}
			np.Management.AutoUpgrade = true
		}
	}

	return request
}

// newClusterReleaseChannel returns the release channel to set on a cluster create
// request. It expects the channel on the config to already be resolved by
// releaseChannelForCreate. An empty or UNSPECIFIED channel means that no release
// channel is sent at all, since GKE rejects both an omitted and an UNSPECIFIED
// channel for the cluster types that require one.
func newClusterReleaseChannel(config *gkev1.GKEClusterConfig) *gkeapi.ReleaseChannel {
	channel := normalizeReleaseChannel(config.Spec.ReleaseChannel)
	if channel == ReleaseChannelUnspecified {
		return nil
	}

	return &gkeapi.ReleaseChannel{
		Channel: channel,
	}
}

// normalizeReleaseChannel converts a configured release channel into a GKE
// release channel value. An unset, empty or "none" channel is normalized to
// UNSPECIFIED, which represents "No channel".
func normalizeReleaseChannel(configured *string) string {
	channel := strings.ToUpper(strings.TrimSpace(utils.StringValue(configured)))
	if channel == "" || channel == ReleaseChannelNone {
		return ReleaseChannelUnspecified
	}
	return channel
}

// DesiredReleaseChannel returns the release channel the cluster should end up
// enrolled in. UNSPECIFIED means the cluster should not be enrolled in any
// release channel.
func DesiredReleaseChannel(config *gkev1.GKEClusterConfig) string {
	return normalizeReleaseChannel(config.Spec.ReleaseChannel)
}

// CanUnenrollFromReleaseChannel indicates whether a cluster is allowed to leave
// its release channel. Autopilot clusters are always managed by a release
// channel and cannot opt out.
func CanUnenrollFromReleaseChannel(config *gkev1.GKEClusterConfig) bool {
	return config.Spec.AutopilotConfig == nil || !config.Spec.AutopilotConfig.Enabled
}

// releaseChannelForCreate returns the release channel to enroll a cluster in
// when it is created.
//
// GKE no longer allows creating a cluster that is not enrolled in a release
// channel, and rejects both an omitted release channel and an explicit
// UNSPECIFIED channel. Clusters that should not be enrolled in a channel are
// therefore created in a temporary channel that supports the requested
// Kubernetes version, and are unenrolled once they are running.
func releaseChannelForCreate(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) (string, error) {
	// Alpha clusters cannot be enrolled in a release channel at all.
	if config.Spec.EnableKubernetesAlpha != nil && *config.Spec.EnableKubernetesAlpha {
		return ReleaseChannelUnspecified, nil
	}

	desired := DesiredReleaseChannel(config)
	if desired != ReleaseChannelUnspecified {
		return desired, nil
	}

	// Autopilot clusters are always enrolled in a release channel and cannot be
	// unenrolled, so let GKE pick its default channel instead of choosing one.
	if !CanUnenrollFromReleaseChannel(config) {
		return ReleaseChannelUnspecified, nil
	}

	return findReleaseChannelForVersion(ctx, gkeClient, config)
}

// findReleaseChannelForVersion returns a release channel that supports the
// Kubernetes version requested by the config.
func findReleaseChannelForVersion(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) (string, error) {
	serverConfig, err := gkeClient.ServerConfigGet(
		ctx, LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)))
	if err != nil {
		return "", fmt.Errorf("error getting GKE server config to select a release channel for cluster [%s (id: %s)]: %w", config.Spec.ClusterName, config.Name, err)
	}

	channels := make(map[string]*gkeapi.ReleaseChannelConfig, len(serverConfig.Channels))
	for _, channel := range serverConfig.Channels {
		if channel == nil {
			continue
		}
		channels[strings.ToUpper(channel.Channel)] = channel
	}

	version := utils.StringValue(config.Spec.KubernetesVersion)
	for _, name := range releaseChannelPreference {
		channel, ok := channels[name]
		if !ok {
			continue
		}
		if version == "" || channelSupportsVersion(channel, version) {
			return name, nil
		}
	}

	if version != "" {
		return "", fmt.Errorf("kubernetes version [%s] is not available in any GKE release channel, and GKE no longer allows creating clusters without a release channel, please select a different version for cluster [%s (id: %s)]", version, config.Spec.ClusterName, config.Name)
	}

	return "", fmt.Errorf("no GKE release channel is available to create cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
}

// channelSupportsVersion reports whether the given release channel can be used
// to create a cluster running the given Kubernetes version. GKE accepts both
// fully qualified versions and version prefixes, so prefixes are matched
// against the versions valid for the channel.
func channelSupportsVersion(channel *gkeapi.ReleaseChannelConfig, version string) bool {
	for _, valid := range channel.ValidVersions {
		if valid == version || strings.HasPrefix(valid, version+".") || strings.HasPrefix(valid, version+"-") {
			return true
		}
	}
	return channel.DefaultVersion == version
}

// validateCreateRequest checks a config for the ability to generate a create request
func validateCreateRequest(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) error {
	if config.Spec.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if config.Spec.Zone == "" && config.Spec.Region == "" {
		return fmt.Errorf("zone or region is required")
	}
	if config.Spec.Zone != "" && config.Spec.Region != "" {
		return fmt.Errorf("only one of zone or region must be specified")
	}
	if config.Spec.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	if len(config.Spec.NodePools) != 0 && config.Spec.AutopilotConfig != nil && config.Spec.AutopilotConfig.Enabled {
		return fmt.Errorf("cannot create node pools for autopilot clusters")
	}

	nodeP := map[string]bool{}
	for _, np := range config.Spec.NodePools {
		if np.Name == nil {
			return fmt.Errorf(cannotBeNilError, "nodePool.name", config.Spec.ClusterName, config.Name)
		}
		if nodeP[*np.Name] {
			return fmt.Errorf("nodePool name [%s] is not unique within the cluster [%s (id: %s)]", utils.StringValue(np.Name), config.Spec.ClusterName, config.Name)
		}
		nodeP[*np.Name] = true

		if np.Autoscaling != nil && np.Autoscaling.Enabled {
			if np.Autoscaling.MinNodeCount < 1 || np.Autoscaling.MaxNodeCount < np.Autoscaling.MinNodeCount {
				return fmt.Errorf("minNodeCount in the NodePool [%s] must be >= 1 and <= maxNodeCount within the cluster [%s (id: %s)]", utils.StringValue(np.Name), config.Spec.ClusterName, config.Name)
			}
		}
	}

	if config.Spec.CustomerManagedEncryptionKey != nil {
		if config.Spec.CustomerManagedEncryptionKey.RingName == "" ||
			config.Spec.CustomerManagedEncryptionKey.KeyName == "" {
			return fmt.Errorf("ringName and keyName are required to enable boot disk encryption for Node Pools within the cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
		}
	}

	operation, err := gkeClient.ClusterList(
		ctx, LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)))
	if err != nil {
		return err
	}

	for _, cluster := range operation.Clusters {
		if cluster.Name == config.Spec.ClusterName {
			return fmt.Errorf("cannot create cluster [%s (id: %s)] because a cluster in GKE exists with the same name, please delete and recreate with a different name", config.Spec.ClusterName, config.Name)
		}
	}

	if config.Spec.Imported {
		// Validation from here on out is for nilable attributes, not required for imported clusters
		return nil
	}

	if config.Spec.EnableKubernetesAlpha == nil {
		return fmt.Errorf(cannotBeNilError, "enableKubernetesAlpha", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.KubernetesVersion == nil {
		return fmt.Errorf(cannotBeNilError, "kubernetesVersion", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.ClusterIpv4CidrBlock == nil {
		return fmt.Errorf(cannotBeNilError, "clusterIpv4CidrBlock", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.ClusterAddons == nil {
		return fmt.Errorf(cannotBeNilError, "clusterAddons", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.IPAllocationPolicy == nil {
		return fmt.Errorf(cannotBeNilError, "ipAllocationPolicy", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.LoggingService == nil {
		return fmt.Errorf(cannotBeNilError, "loggingService", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Network == nil {
		return fmt.Errorf(cannotBeNilError, "network", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Subnetwork == nil {
		return fmt.Errorf(cannotBeNilError, "subnetwork", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.NetworkPolicyEnabled == nil {
		return fmt.Errorf(cannotBeNilError, "networkPolicyEnabled", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.PrivateClusterConfig == nil {
		return fmt.Errorf(cannotBeNilError, "privateClusterConfig", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.PrivateClusterConfig.EnablePrivateEndpoint && !config.Spec.PrivateClusterConfig.EnablePrivateNodes {
		return fmt.Errorf("private endpoint requires private nodes for cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.MasterAuthorizedNetworksConfig == nil {
		return fmt.Errorf(cannotBeNilError, "masterAuthorizedNetworksConfig", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.MonitoringService == nil {
		return fmt.Errorf(cannotBeNilError, "monitoringService", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Locations == nil {
		return fmt.Errorf(cannotBeNilError, "locations", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.MaintenanceWindow == nil {
		return fmt.Errorf(cannotBeNilError, "maintenanceWindow", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Labels == nil {
		return fmt.Errorf(cannotBeNilError, "labels", config.Spec.ClusterName, config.Name)
	}

	for np := range config.Spec.NodePools {
		if err = validateNodePoolCreateRequest(&config.Spec.NodePools[np], config); err != nil {
			return err
		}
	}

	return nil
}

func validateNodePoolCreateRequest(np *gkev1.GKENodePoolConfig, config *gkev1.GKEClusterConfig) error {
	clusterErr := cannotBeNilError
	nodePoolErr := cannotBeNilForNodePoolError
	clusterName := config.Spec.ClusterName
	if np.Name == nil {
		return fmt.Errorf(clusterErr, "nodePool.name", clusterName, config.Name)
	}
	if np.Version == nil {
		return fmt.Errorf(nodePoolErr, "version", *np.Name, clusterName, config.Name)
	}
	if np.Autoscaling == nil {
		return fmt.Errorf(nodePoolErr, "autoscaling", *np.Name, clusterName, config.Name)
	}
	if np.InitialNodeCount == nil {
		return fmt.Errorf(nodePoolErr, "initialNodeCount", *np.Name, clusterName, config.Name)
	}
	if np.MaxPodsConstraint == nil && config.Spec.IPAllocationPolicy != nil && config.Spec.IPAllocationPolicy.UseIPAliases {
		return fmt.Errorf(nodePoolErr, "maxPodsConstraint", *np.Name, clusterName, config.Name)
	}
	if np.Config == nil {
		return fmt.Errorf(nodePoolErr, "config", *np.Name, clusterName, config.Name)
	}
	if np.Management == nil {
		return fmt.Errorf(nodePoolErr, "management", *np.Name, clusterName, config.Name)
	}

	rxEmail := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-z]{2,}$`)
	if np.Config.ServiceAccount != "" && np.Config.ServiceAccount != "default" && !rxEmail.MatchString(np.Config.ServiceAccount) {
		return fmt.Errorf("field [%s] must either be an empty string, 'default' or set to a valid email address for nodepool [%s] in non-nil cluster [%s (id: %s)]", "serviceAccount", *np.Name, clusterName, config.Name)
	}
	return nil
}

func newNodePoolCreateRequest(np *gkev1.GKENodePoolConfig, config *gkev1.GKEClusterConfig) (*gkeapi.CreateNodePoolRequest, error) {
	parent := ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName)
	request := &gkeapi.CreateNodePoolRequest{
		Parent:   parent,
		NodePool: newGKENodePoolFromConfig(np, config),
	}
	return request, nil
}

func newGKENodePoolFromConfig(np *gkev1.GKENodePoolConfig, config *gkev1.GKEClusterConfig) *gkeapi.NodePool {
	taints := make([]*gkeapi.NodeTaint, 0, len(np.Config.Taints))
	for _, t := range np.Config.Taints {
		taints = append(taints, &gkeapi.NodeTaint{
			Effect: t.Effect,
			Key:    t.Key,
			Value:  t.Value,
		})
	}
	ret := &gkeapi.NodePool{
		Name: *np.Name,
		Autoscaling: &gkeapi.NodePoolAutoscaling{
			Enabled:      np.Autoscaling.Enabled,
			MaxNodeCount: np.Autoscaling.MaxNodeCount,
			MinNodeCount: np.Autoscaling.MinNodeCount,
		},
		InitialNodeCount: *np.InitialNodeCount,
		Config: &gkeapi.NodeConfig{
			DiskSizeGb:     np.Config.DiskSizeGb,
			DiskType:       np.Config.DiskType,
			ImageType:      np.Config.ImageType,
			Labels:         np.Config.Labels,
			LocalSsdCount:  np.Config.LocalSsdCount,
			MachineType:    np.Config.MachineType,
			OauthScopes:    np.Config.OauthScopes,
			Preemptible:    np.Config.Preemptible,
			Tags:           np.Config.Tags,
			Taints:         taints,
			ServiceAccount: np.Config.ServiceAccount,
		},
		Version: *np.Version,
		Management: &gkeapi.NodeManagement{
			AutoRepair:  np.Management.AutoRepair,
			AutoUpgrade: np.Management.AutoUpgrade,
		},
	}
	if config.Spec.CustomerManagedEncryptionKey != nil &&
		config.Spec.CustomerManagedEncryptionKey.RingName != "" &&
		config.Spec.CustomerManagedEncryptionKey.KeyName != "" {
		ret.Config.BootDiskKmsKey = BootDiskRRN(
			config.Spec.ProjectID,
			Location(config.Spec.Region, config.Spec.Zone),
			config.Spec.CustomerManagedEncryptionKey.RingName,
			config.Spec.CustomerManagedEncryptionKey.KeyName,
		)
	}
	if config.Spec.IPAllocationPolicy != nil && config.Spec.IPAllocationPolicy.UseIPAliases {
		ret.MaxPodsConstraint = &gkeapi.MaxPodsConstraint{
			MaxPodsPerNode: *np.MaxPodsConstraint,
		}
	}
	return ret
}
