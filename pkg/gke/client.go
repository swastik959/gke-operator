package gke

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rancher/gke-operator/pkg/gke/services"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gkeapi "google.golang.org/api/container/v1"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

// Credential type constants. These mirror the enum exposed on
// GKEClusterConfigSpec.GoogleCredentialType and are kept here so that lower
// level GKE client code does not have to import the typed API package.
const (
	// CredentialTypeServiceAccountKey selects a long-lived service account
	// JSON key. This is the legacy behavior and remains the default for
	// backward compatibility.
	CredentialTypeServiceAccountKey = "serviceAccountKey"

	// CredentialTypeWorkloadIdentityFederation selects a GCP Workload Identity
	// Federation `external_account` JSON document.
	CredentialTypeWorkloadIdentityFederation = "workloadIdentityFederation"

	// CredentialTypeApplicationDefault uses Application Default Credentials
	// from the environment the operator runs in (e.g. GKE Workload Identity).
	CredentialTypeApplicationDefault = "applicationDefault"
)

// AuthOptions describes how to build a GCP TokenSource / Service client.
type AuthOptions struct {
	// CredentialType is one of the CredentialType* constants. An empty string
	// is treated as CredentialTypeServiceAccountKey for backward compatibility.
	CredentialType string

	// Credential is the raw JSON credential document. It must be empty when
	// CredentialType is CredentialTypeApplicationDefault and non-empty
	// otherwise.
	Credential string

	// ImpersonateServiceAccount, if non-empty, causes the resulting
	// TokenSource to be wrapped with an impersonated credential producing
	// short-lived (1h) tokens for the named service account.
	ImpersonateServiceAccount string
}

// credentialEnvelope is the minimal shape of a GCP JSON credential blob used
// to detect which credential type it represents.
type credentialEnvelope struct {
	Type string `json:"type"`
}

// GetGKEClient accepts a JSON credential string and returns a Service client.
//
// Deprecated: prefer GetGKEClientWithOptions, which understands credential
// types beyond service account JSON keys.
func GetGKEClient(ctx context.Context, credential string) (*gkeapi.Service, error) {
	return GetGKEClientWithOptions(ctx, AuthOptions{Credential: credential})
}

// GetGKEClientWithOptions returns a GKE Service client built from the supplied
// AuthOptions.
func GetGKEClientWithOptions(ctx context.Context, opts AuthOptions) (*gkeapi.Service, error) {
	ts, err := GetTokenSourceWithOptions(ctx, opts)
	if err != nil {
		return nil, err
	}
	return getServiceClientWithTokenSource(ctx, ts)
}

// GetGKEClusterClient returns a high-level GKEClusterService using a JSON
// credential string.
//
// Deprecated: prefer GetGKEClusterClientWithOptions.
func GetGKEClusterClient(ctx context.Context, credential string) (services.GKEClusterService, error) {
	return GetGKEClusterClientWithOptions(ctx, AuthOptions{Credential: credential})
}

// GetGKEClusterClientWithOptions returns a high-level GKEClusterService built
// from the supplied AuthOptions.
func GetGKEClusterClientWithOptions(ctx context.Context, opts AuthOptions) (services.GKEClusterService, error) {
	ts, err := GetTokenSourceWithOptions(ctx, opts)
	if err != nil {
		return nil, err
	}
	return services.NewGKEClusterService(ctx, ts)
}

func getServiceClientWithTokenSource(ctx context.Context, ts oauth2.TokenSource) (*gkeapi.Service, error) {
	return gkeapi.NewService(ctx, option.WithHTTPClient(oauth2.NewClient(ctx, ts)))
}

// GetTokenSource returns an oauth2.TokenSource derived from a JSON credential.
// The credential may be either a `service_account` or `external_account`
// JSON document; the underlying google library detects the type.
//
// Deprecated: prefer GetTokenSourceWithOptions.
func GetTokenSource(ctx context.Context, credential string) (oauth2.TokenSource, error) {
	return GetTokenSourceWithOptions(ctx, AuthOptions{Credential: credential})
}

// GetTokenSourceWithOptions returns an oauth2.TokenSource constructed
// according to opts. It supports service-account JSON keys, Workload
// Identity Federation external_account JSON, and Application Default
// Credentials, and can optionally wrap the result with service-account
// impersonation.
func GetTokenSourceWithOptions(ctx context.Context, opts AuthOptions) (oauth2.TokenSource, error) {
	credType := opts.CredentialType
	if credType == "" {
		credType = CredentialTypeServiceAccountKey
	}

	var (
		ts  oauth2.TokenSource
		err error
	)

	switch credType {
	case CredentialTypeApplicationDefault:
		if opts.Credential != "" {
			return nil, fmt.Errorf("credential data must not be provided when using credential type %q", credType)
		}
		creds, fdErr := google.FindDefaultCredentials(ctx, gkeapi.CloudPlatformScope)
		if fdErr != nil {
			return nil, fmt.Errorf("error finding application default credentials: %w", fdErr)
		}
		ts = creds.TokenSource

	case CredentialTypeServiceAccountKey, CredentialTypeWorkloadIdentityFederation:
		if opts.Credential == "" {
			return nil, fmt.Errorf("credential data is required for credential type %q", credType)
		}
		if vErr := validateCredentialJSONType(opts.Credential, credType); vErr != nil {
			return nil, vErr
		}
		creds, cErr := google.CredentialsFromJSON(ctx, []byte(opts.Credential), gkeapi.CloudPlatformScope)
		if cErr != nil {
			return nil, cErr
		}
		ts = creds.TokenSource

	default:
		return nil, fmt.Errorf("unsupported google credential type %q", credType)
	}

	if opts.ImpersonateServiceAccount != "" {
		ts, err = impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: opts.ImpersonateServiceAccount,
			Scopes:          []string{gkeapi.CloudPlatformScope},
		}, option.WithTokenSource(ts))
		if err != nil {
			return nil, fmt.Errorf("error building impersonated token source for %q: %w", opts.ImpersonateServiceAccount, err)
		}
	}

	return ts, nil
}

// validateCredentialJSONType ensures the JSON credential blob's `type` field
// matches what the chosen credential mode expects. This catches user errors
// such as pasting a service-account key into a workload-identity-federation
// configuration (and vice versa) before the token exchange is attempted.
func validateCredentialJSONType(credential, credType string) error {
	var env credentialEnvelope
	if err := json.Unmarshal([]byte(credential), &env); err != nil {
		return fmt.Errorf("error parsing google credential JSON: %w", err)
	}
	switch credType {
	case CredentialTypeServiceAccountKey:
		if env.Type != "" && env.Type != "service_account" {
			return fmt.Errorf("credential type %q requires a JSON credential with \"type\": \"service_account\", got %q", credType, env.Type)
		}
	case CredentialTypeWorkloadIdentityFederation:
		if env.Type != "external_account" {
			return fmt.Errorf("credential type %q requires a JSON credential with \"type\": \"external_account\", got %q", credType, env.Type)
		}
	}
	return nil
}
