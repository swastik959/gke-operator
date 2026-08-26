package gke

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services/mock_services"
	gkeapi "google.golang.org/api/container/v1"
)

var _ = Describe("CreateCluster", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.25.12-gke.200"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true
		nodePoolName       = "test-node-pool"
		initialNodeCount   = int64(3)
		maxPodsConstraint  = int64(110)
		config             = &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{
				Region:                "test-region",
				ProjectID:             "test-project",
				ClusterName:           "test-cluster",
				Locations:             []string{""},
				Labels:                map[string]string{"test": "test"},
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolTrue,
				Network:               &networkName,
				Subnetwork:            &subnetworkName,
				NetworkPolicyEnabled:  &boolTrue,
				MaintenanceWindow:     &emptyString,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: false,
				},
			},
		}
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create cluster", func() {
		createClusterRequest := NewClusterCreateRequest(config)
		clusterServiceMock.EXPECT().
			ClusterCreate(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
				createClusterRequest).
			Return(&gkeapi.Operation{}, nil)

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).ToNot(HaveOccurred())

		clusterServiceMock.EXPECT().
			ClusterGet(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone),
					config.Spec.ClusterName)).
			Return(
				&gkeapi.Cluster{
					Name: "test-cluster",
				}, nil)

		managedCluster, err := GetCluster(ctx, clusterServiceMock, &config.Spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(managedCluster.Name).To(Equal(config.Spec.ClusterName))
	})

	It("should successfully create cluster with customer managment encryption key", func() {
		config.Spec.CustomerManagedEncryptionKey = &gkev1.CMEKConfig{
			KeyName:  "test-key",
			RingName: "test-keyring",
		}
		createClusterRequest := NewClusterCreateRequest(config)
		clusterServiceMock.EXPECT().
			ClusterCreate(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
				createClusterRequest).
			Return(&gkeapi.Operation{}, nil)

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).ToNot(HaveOccurred())

		clusterServiceMock.EXPECT().
			ClusterGet(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone),
					config.Spec.ClusterName)).
			Return(
				&gkeapi.Cluster{
					Name: "test-cluster",
				}, nil)

		managedCluster, err := GetCluster(ctx, clusterServiceMock, &config.Spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(managedCluster.Name).To(Equal(config.Spec.ClusterName))
	})

	It("should fail to create cluster", func() {
		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(
				&gkeapi.ListClustersResponse{
					Clusters: []*gkeapi.Cluster{
						{
							Name: "test-cluster",
						},
					},
				}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
	})

	It("should successfully create autopilot cluster", func() {
		config.Spec.ClusterName = "test-autopilot-cluster"
		config.Spec.AutopilotConfig = &gkev1.GKEAutopilotConfig{
			Enabled: true,
		}

		createClusterRequest := NewClusterCreateRequest(config)
		clusterServiceMock.EXPECT().
			ClusterCreate(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
				createClusterRequest).
			Return(&gkeapi.Operation{}, nil)

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).ToNot(HaveOccurred())

		clusterServiceMock.EXPECT().
			ClusterGet(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone),
					config.Spec.ClusterName)).
			Return(
				&gkeapi.Cluster{
					Name: "test-autopilot-cluster",
				}, nil)

		managedCluster, err := GetCluster(ctx, clusterServiceMock, &config.Spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(managedCluster.Name).To(Equal(config.Spec.ClusterName))
	})

	It("should fail create cluster with customer managment encryption key", func() {
		config.Spec.CustomerManagedEncryptionKey = &gkev1.CMEKConfig{
			KeyName: "test-key",
		}
		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to create autopilot cluster with nodepools", func() {
		config.Spec.ClusterName = "test-autopilot-cluster"
		config.Spec.AutopilotConfig = &gkev1.GKEAutopilotConfig{
			Enabled: true,
		}

		config.Spec.NodePools = []gkev1.GKENodePoolConfig{
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				Version:           &k8sVersion,
				MaxPodsConstraint: &maxPodsConstraint,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      true,
					MinNodeCount: 3,
					MaxNodeCount: 5,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
		}

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to create cluster with duplicated nodepool names", func() {
		config.Spec.NodePools = []gkev1.GKENodePoolConfig{
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				Version:           &k8sVersion,
				MaxPodsConstraint: &maxPodsConstraint,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      true,
					MinNodeCount: 3,
					MaxNodeCount: 5,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				Version:           &k8sVersion,
				MaxPodsConstraint: &maxPodsConstraint,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      true,
					MinNodeCount: 3,
					MaxNodeCount: 5,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
		}
		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("CreateNodePool", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.25.12-gke.200"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true

		nodePoolName      = "test-node-pool"
		initialNodeCount  = int64(3)
		maxPodsConstraint = int64(110)
		nodePoolConfig    = &gkev1.GKENodePoolConfig{
			Name:              &nodePoolName,
			InitialNodeCount:  &initialNodeCount,
			Version:           &k8sVersion,
			MaxPodsConstraint: &maxPodsConstraint,
			Config:            &gkev1.GKENodeConfig{},
			Autoscaling: &gkev1.GKENodePoolAutoscaling{
				Enabled:      true,
				MinNodeCount: 3,
				MaxNodeCount: 5,
			},
			Management: &gkev1.GKENodePoolManagement{
				AutoRepair:  true,
				AutoUpgrade: true,
			},
		}

		config = &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{
				Region:                "test-region",
				ProjectID:             "test-project",
				ClusterName:           "test-cluster",
				Locations:             []string{""},
				Labels:                map[string]string{"test": "test"},
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolTrue,
				Network:               &networkName,
				Subnetwork:            &subnetworkName,
				NetworkPolicyEnabled:  &boolTrue,
				MaintenanceWindow:     &emptyString,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: false,
				},
			},
		}
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create cluster and node pool", func() {
		createClusterRequest := NewClusterCreateRequest(config)
		clusterServiceMock.EXPECT().
			ClusterCreate(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
				createClusterRequest).
			Return(&gkeapi.Operation{}, nil)

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).ToNot(HaveOccurred())

		createNodePoolRequest, err := newNodePoolCreateRequest(nodePoolConfig, config)
		Expect(err).ToNot(HaveOccurred())
		clusterServiceMock.EXPECT().
			NodePoolCreate(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				createNodePoolRequest).
			Return(&gkeapi.Operation{}, nil)

		status, err := CreateNodePool(ctx, clusterServiceMock, config, nodePoolConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(Changed))
	})
	It("shouldn't successfully create cluster and node pool", func() {
		testNodePoolConfig := &gkev1.GKENodePoolConfig{}
		status, err := CreateNodePool(ctx, clusterServiceMock, config, testNodePoolConfig)
		Expect(err).To(HaveOccurred())
		Expect(status).To(Equal(NotChanged))
	})
})

var _ = Describe("release channel on create", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.25.12-gke.200"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true
		boolFalse          = false
		npName             = "test-node-pool"
		npInitialNodeCount = int64(3)
		npMaxPods          = int64(110)
		config             *gkev1.GKEClusterConfig
		serverConfig       *gkeapi.ServerConfig
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
		serverConfig = &gkeapi.ServerConfig{
			Channels: []*gkeapi.ReleaseChannelConfig{
				{
					Channel:       "RAPID",
					ValidVersions: []string{"1.30.1-gke.100"},
				},
				{
					Channel:       "REGULAR",
					ValidVersions: []string{"1.29.1-gke.100", k8sVersion},
				},
				{
					Channel:       "STABLE",
					ValidVersions: []string{"1.28.1-gke.100"},
				},
			},
		}
		config = &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{
				Region:                "test-region",
				ProjectID:             "test-project",
				ClusterName:           "test-cluster",
				Locations:             []string{""},
				Labels:                map[string]string{"test": "test"},
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolFalse,
				Network:               &networkName,
				Subnetwork:            &subnetworkName,
				NetworkPolicyEnabled:  &boolTrue,
				MaintenanceWindow:     &emptyString,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: false,
				},
				NodePools: []gkev1.GKENodePoolConfig{
					{
						Name:              &npName,
						InitialNodeCount:  &npInitialNodeCount,
						Version:           &k8sVersion,
						MaxPodsConstraint: &npMaxPods,
						Config:            &gkev1.GKENodeConfig{},
						Autoscaling:       &gkev1.GKENodePoolAutoscaling{},
						Management: &gkev1.GKENodePoolManagement{
							AutoUpgrade: false,
						},
					},
				},
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	expectCreate := func() *gkeapi.CreateClusterRequest {
		var request *gkeapi.CreateClusterRequest
		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)
		clusterServiceMock.EXPECT().
			ClusterCreate(ctx, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, req *gkeapi.CreateClusterRequest) (*gkeapi.Operation, error) {
				request = req
				return &gkeapi.Operation{}, nil
			})
		Expect(Create(ctx, clusterServiceMock, config)).To(Succeed())
		return request
	}

	It("should enroll in a channel that supports the requested version when no channel is configured", func() {
		clusterServiceMock.EXPECT().
			ServerConfigGet(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(serverConfig, nil)

		request := expectCreate()
		Expect(request.Cluster.ReleaseChannel).ToNot(BeNil())
		Expect(request.Cluster.ReleaseChannel.Channel).To(Equal(ReleaseChannelRegular))
	})

	It("should force node auto-upgrade while enrolled in a release channel", func() {
		clusterServiceMock.EXPECT().
			ServerConfigGet(ctx, gomock.Any()).
			Return(serverConfig, nil)

		request := expectCreate()
		Expect(request.Cluster.NodePools).To(HaveLen(1))
		Expect(request.Cluster.NodePools[0].Management.AutoUpgrade).To(BeTrue())
		// The configured value is left untouched so it is restored after the
		// cluster is unenrolled from the release channel.
		Expect(config.Spec.NodePools[0].Management.AutoUpgrade).To(BeFalse())
	})

	It("should prefer the most mature channel that supports the requested version", func() {
		stableVersion := "1.28.1-gke.100"
		config.Spec.KubernetesVersion = &stableVersion
		config.Spec.NodePools[0].Version = &stableVersion
		clusterServiceMock.EXPECT().
			ServerConfigGet(ctx, gomock.Any()).
			Return(serverConfig, nil)

		request := expectCreate()
		Expect(request.Cluster.ReleaseChannel.Channel).To(Equal(ReleaseChannelStable))
	})

	It("should use the configured channel without looking up the server config", func() {
		channel := "rapid"
		config.Spec.ReleaseChannel = &channel

		request := expectCreate()
		Expect(request.Cluster.ReleaseChannel.Channel).To(Equal(ReleaseChannelRapid))
	})

	It("should not enroll alpha clusters in a release channel", func() {
		config.Spec.EnableKubernetesAlpha = &boolTrue

		request := expectCreate()
		Expect(request.Cluster.ReleaseChannel).To(BeNil())
		Expect(request.Cluster.NodePools[0].Management.AutoUpgrade).To(BeFalse())
	})

	It("should let GKE pick the channel for autopilot clusters", func() {
		config.Spec.AutopilotConfig = &gkev1.GKEAutopilotConfig{Enabled: true}
		config.Spec.NodePools = nil

		request := expectCreate()
		Expect(request.Cluster.ReleaseChannel).To(BeNil())
	})

	It("should fail when no release channel supports the requested version", func() {
		unsupported := "1.20.1-gke.100"
		config.Spec.KubernetesVersion = &unsupported
		config.Spec.NodePools[0].Version = &unsupported
		clusterServiceMock.EXPECT().
			ClusterList(ctx, gomock.Any()).
			Return(&gkeapi.ListClustersResponse{}, nil)
		clusterServiceMock.EXPECT().
			ServerConfigGet(ctx, gomock.Any()).
			Return(serverConfig, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not available in any GKE release channel"))
	})

	It("should return the error when the server config cannot be read", func() {
		clusterServiceMock.EXPECT().
			ClusterList(ctx, gomock.Any()).
			Return(&gkeapi.ListClustersResponse{}, nil)
		clusterServiceMock.EXPECT().
			ServerConfigGet(ctx, gomock.Any()).
			Return(nil, errors.New("boom"))

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("error getting GKE server config"))
	})
})

var _ = Describe("DesiredReleaseChannel", func() {
	It("should normalize an unset channel to unspecified", func() {
		Expect(DesiredReleaseChannel(&gkev1.GKEClusterConfig{})).To(Equal(ReleaseChannelUnspecified))
	})

	It("should normalize none to unspecified", func() {
		for _, in := range []string{"none", "None", " NONE ", "", "unspecified"} {
			channel := in
			config := &gkev1.GKEClusterConfig{
				Spec: gkev1.GKEClusterConfigSpec{ReleaseChannel: &channel},
			}
			Expect(DesiredReleaseChannel(config)).To(Equal(ReleaseChannelUnspecified))
		}
	})

	It("should upper case a configured channel", func() {
		channel := "regular"
		config := &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{ReleaseChannel: &channel},
		}
		Expect(DesiredReleaseChannel(config)).To(Equal(ReleaseChannelRegular))
	})
})
