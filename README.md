[![Nightly e2e tests](https://github.com/rancher/gke-operator/actions/workflows/e2e-latest-rancher.yaml/badge.svg?branch=main)](https://github.com/rancher/gke-operator/actions/workflows/e2e-latest-rancher.yaml)

# gke-operator

The GKE operator is a controller for Kubernetes Custom Resource Definitions (CRDs) that manages cluster provisioning in Google Kubernetes Engine. It uses a GKEClusterConfig defined by a CRD.

## Credentials

The operator supports three ways of obtaining Google Cloud credentials, selected via the `spec.googleCredentialType` field on a `GKEClusterConfig`:

| `googleCredentialType` | Secret required | Notes |
| --- | --- | --- |
| `serviceAccountKey` (default, **deprecated**) | yes — `service_account` JSON | Long-lived static key. Provided for backward compatibility; a deprecation warning is logged on every reconcile. |
| `workloadIdentityFederation` | yes — `external_account` JSON | Workload Identity Federation. Short-lived, automatically rotated access tokens. Recommended for GitHub Actions / GitLab CI / Azure AD / OIDC providers. |
| `applicationDefault` | no | Uses Application Default Credentials from the environment. When the operator pod runs on GKE with Workload Identity bound to its KSA, this is fully keyless. |

When set, `spec.googleCredentialType` defaults to `serviceAccountKey` so existing `GKEClusterConfig` resources continue to work unchanged.

The credential JSON (for the first two modes) is read from the secret referenced by `spec.googleCredentialSecret` (`namespace:name`) under the key `googlecredentialConfig-authEncodedJson`, the same key Rancher's cloud-credential machinery already uses. The operator validates that the JSON `type` field matches the selected mode.

### Service-account impersonation

For any of the above modes, `spec.impersonateServiceAccount` may be set to a service-account email. When present, the base credentials are wrapped with `google.golang.org/api/impersonate` to mint short-lived (1-hour) tokens for that account. This is a useful stepping stone toward keyless operation while a fleet still ships long-lived keys.

A complete example using Workload Identity Federation is available at [`examples/cluster-wif.yaml`](examples/cluster-wif.yaml).

## Build

Operator binary can be built using the following command:

```sh
    make operator
```

## Deploy operator from source

You can use the following command to deploy a Kind cluster with Rancher manager and operator:

```sh
    make kind-deploy-operator
```

After this, you can also scale down operator deployment and run it from a local binary.

## Tests

To run unit tests use the following command:

```sh
    make test
```

## E2E

We run e2e tests after every merged PR and periodically every 24 hours. They are triggered by a [Github action](https://github.com/rancher/gke-operator/blob/main/.github/workflows/e2e-latest-rancher.yaml)

For running e2e tests:

1. Set `GKE_PROJECT_ID` and `GKE_CREDENTIALS` environment variables:

```sh
    export GKE_PROJECT_ID="replace-with-your-value"
    export GKE_CREDENTIALS=$( cat /path/to/gke-credentials.json )
```

2. and finally run:

```sh
    make kind-e2e-tests
```

This will setup a kind cluster locally, and the e2e tests will be run against where it will:

    * deploy rancher and cert-manager
    * deploy gke operator and operator CRD charts
    * create gke credentials secret
    * create a cluster in GKE
    * wait for cluster to be ready
    * clean up cluster


Once e2e tests are completed, the local kind cluster can also be deleted by running:

```bash
    make delete-local-kind-cluster
```

## Release

### When should I release?

A KEv2 operator should be released if:

* There have been several commits since the last release,
* You need to pull in an update/bug fix/backend code to unblock UI for a feature enhancement in Rancher
* The operator needs to be unRC for a Rancher release

### How do I release?

Tag the latest commit on the `master` branch. For example, if latest tag is:
* `v1.1.3-rc1` you should tag `v1.1.3-rc2`.
* `v1.1.3` you should tag `v1.1.4-rc1`.

```bash
# Get the latest upstream changes
# Note: `upstream` must be the remote pointing to `git@github.com:rancher/gke-operator.git`.
git pull upstream master --tags

# Export the tag of the release to be cut, e.g.:
export RELEASE_TAG=v1.1.3-rc2

# Create tags locally
git tag -s -a ${RELEASE_TAG} -m ${RELEASE_TAG}

# Push tags
# Note: `upstream` must be the remote pointing to `git@github.com:rancher/gke-operator.git`.
git push upstream ${RELEASE_TAG}
```

After pushing the release tag, you need to run 2 Github Actions. You can find them in the Actions tab of the repo:

* [Update GKE operator in rancher/rancher](https://github.com/rancher/gke-operator/actions/workflows/update-rancher-dep.yaml) - This action will update the GKE operator in rancher/rancher repo. It will bump go dependencies.
* [Update GKE operator in rancher/charts](https://github.com/rancher/gke-operator/actions/workflows/update-rancher-charts.yaml) - This action will update the GKE operator in rancher/charts repo. It will bump the chart version.

### How do I unRC?

unRC is the process of removing the rc from a KEv2 operator tag and means the released version is stable and ready for use. Release the KEv2 operator but instead of bumping the rc, remove the rc. For example, if the latest release of GKE operator is:
* `v1.1.3-rc1`, release the next version without the rc which would be `v1.1.3`.
* `v1.1.3`, that has no rc so release that version or `v1.1.4` if updates are available.
