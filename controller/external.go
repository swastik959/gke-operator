package controller

import (
	"context"
	"fmt"
	"strings"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke"
	wranglerv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"golang.org/x/oauth2"
	gkeapi "google.golang.org/api/container/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func parseCredential(ref string) (namespace string, name string) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

// credentialType returns the configured google credential type, defaulting to
// the legacy service-account-key behaviour when unspecified.
func credentialType(configSpec *gkev1.GKEClusterConfigSpec) string {
	if configSpec == nil || configSpec.GoogleCredentialType == "" {
		return gke.CredentialTypeServiceAccountKey
	}
	return configSpec.GoogleCredentialType
}

// ValidateCredentialSpec performs lightweight validation of the credential
// related fields on a GKEClusterConfigSpec. It is intentionally tolerant of
// missing data (returning nil) when the chosen credential type does not
// require a secret.
func ValidateCredentialSpec(configSpec *gkev1.GKEClusterConfigSpec) error {
	if configSpec == nil {
		return nil
	}
	switch credentialType(configSpec) {
	case gke.CredentialTypeApplicationDefault:
		if configSpec.GoogleCredentialSecret != "" {
			return fmt.Errorf("googleCredentialSecret must be empty when googleCredentialType is %q", gke.CredentialTypeApplicationDefault)
		}
	case gke.CredentialTypeServiceAccountKey, gke.CredentialTypeWorkloadIdentityFederation:
		if configSpec.GoogleCredentialSecret == "" {
			return fmt.Errorf("googleCredentialSecret is required for googleCredentialType %q", credentialType(configSpec))
		}
	default:
		return fmt.Errorf("unsupported googleCredentialType %q", configSpec.GoogleCredentialType)
	}
	return nil
}

// GetSecret returns the raw JSON credential document referenced by the
// config spec, or an empty string when the configured credential type does
// not require a secret (e.g. applicationDefault).
func GetSecret(_ context.Context, secretsClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (string, error) {
	if err := ValidateCredentialSpec(configSpec); err != nil {
		return "", err
	}
	if credentialType(configSpec) == gke.CredentialTypeApplicationDefault {
		return "", nil
	}
	ns, id := parseCredential(configSpec.GoogleCredentialSecret)
	secret, err := secretsClient.Get(ns, id, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	dataBytes, ok := secret.Data["googlecredentialConfig-authEncodedJson"]
	if !ok {
		return "", fmt.Errorf("could not read malformed cloud credential secret %s from namespace %s", id, ns)
	}
	return string(dataBytes), nil
}

// authOptionsFor builds the gke.AuthOptions corresponding to a config spec.
func authOptionsFor(configSpec *gkev1.GKEClusterConfigSpec, credential string) gke.AuthOptions {
	if configSpec == nil {
		return gke.AuthOptions{Credential: credential}
	}
	return gke.AuthOptions{
		CredentialType:            credentialType(configSpec),
		Credential:                credential,
		ImpersonateServiceAccount: configSpec.ImpersonateServiceAccount,
	}
}

func GetCluster(ctx context.Context, secretsClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (*gkeapi.Cluster, error) {
	cred, err := GetSecret(ctx, secretsClient, configSpec)
	if err != nil {
		return nil, err
	}
	gkeClient, err := gke.GetGKEClusterClientWithOptions(ctx, authOptionsFor(configSpec, cred))
	if err != nil {
		return nil, err
	}
	return gke.GetCluster(ctx, gkeClient, configSpec)
}

func GetTokenSource(ctx context.Context, secretsClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (oauth2.TokenSource, error) {
	cred, err := GetSecret(ctx, secretsClient, configSpec)
	if err != nil {
		return nil, fmt.Errorf("error getting secret: %w", err)
	}
	ts, err := gke.GetTokenSourceWithOptions(ctx, authOptionsFor(configSpec, cred))
	if err != nil {
		return nil, fmt.Errorf("error getting oauth2 token: %w", err)
	}
	return ts, nil
}

// BuildUpstreamClusterState creates an GKEClusterConfigSpec (spec for the GKE cluster state) from the existing
// cluster configuration.
func BuildUpstreamClusterState(ctx context.Context, secretsCache wranglerv1.SecretCache, secretClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (*gkev1.GKEClusterConfigSpec, error) {
	cred, err := GetSecret(ctx, secretClient, configSpec)
	if err != nil {
		return nil, err
	}
	gkeClient, err := gke.GetGKEClusterClientWithOptions(ctx, authOptionsFor(configSpec, cred))
	if err != nil {
		return nil, err
	}
	gkeCluster, err := gke.GetCluster(ctx, gkeClient, configSpec)
	if err != nil {
		return nil, err
	}

	h := Handler{
		secretsCache: secretsCache,
		secrets:      secretClient,
	}
	return h.buildUpstreamClusterState(gkeCluster)
}
