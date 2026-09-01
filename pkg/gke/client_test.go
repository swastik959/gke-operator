package gke

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A syntactically valid (but unusable) service-account JSON document. The
// embedded private key is a freshly generated 1024-bit RSA test key used only
// by these unit tests; no real GCP identity is associated with it.
const testServiceAccountJSON = `{
    "type": "service_account",
    "project_id": "test-project",
    "private_key_id": "1234567890123456",
    "private_key": "-----BEGIN PRIVATE KEY-----\nMIICWwIBAAKBgQCjq3PA6Un07pRRcIJVaKSdj2g2QOWWkjjfAdRqxfvVAfBZtryP\nmUyo/JxBAfRpHc9an9FkOJNH9MPwOQdbxmU2S5zuAsbRmpYXd5q7tIqO++XBTQBf\ng4THdSvHJrzGx9uYzOJfMATiI8mif5DZkblxQMgdzBwqutf+VvzkTbk3nwIDAQAB\nAoGAEvcJAK+HnFQQ56br002+1WsKnk7Cy8HByUWDAaRTXAlPenXMP695zJMI4BeD\n5LJJlqyyLLTJjCr2kV1qVt4UWBjJ4q7qfI0tVwHcK5laM034z8XNbTgknqPxVJaU\nW90B3T+vQIDfu7PDZjxsoKE1BioXSxyL26tR4NMKCrxz0dECQQDWW0lWalFUpHDI\ntG/JOpTX9DFCnrvzqZ6BP0LBn2Ps0XuMQkfiKMOEbxKm5yeB2ULO2giFY+OVdxKO\nXmEQwLgFAkEAw3dUJRL2Rb7PGIJ2nPmnvs6lmEN3jNPaHjLArEe2Rw7kf6XQxt+s\nbVtwO0WJ86NA4Yf3WLBQnN1xpvJTSOK2UwJAYQcHJj+PuvGIP8E1DHAg6bOWDKLP\nTtcLcVOSQxSD5bFY7D8gTKXJAoxIdBYT0vnl/L3Ct6ZkYMZ6NslPxIaHhQJAGZQD\n7tYMZBQUBaEM5H3G9bEU+lfZzRPr9wetLt4zfBj2zb1lFKEwbx8IELmI09kJJHom\nY/Sul9hihvYu79q7AQJAWgx1VbozkvAbedRrd5GCYQH7yBVnQuWA49N4su494qd4\nvLotJ+520U/9JcdtiL+Q3EdjO5Y60eJLX/PIBvZcig==\n-----END PRIVATE KEY-----\n",
    "client_email": "test@example.iam.gserviceaccount.com",
    "client_id": "test",
    "auth_uri": "https://www.example.com",
    "token_uri": "https://www.example.com",
    "auth_provider_x509_cert_url": "https://www.example.com",
    "client_x509_cert_url": "https://www.example.com",
    "universe_domain": "example.com"
}`

// A syntactically valid external_account (Workload Identity Federation) JSON
// document. The referenced credential_source file does not need to exist:
// google.CredentialsFromJSON parses the envelope eagerly but defers any
// actual token exchange until the TokenSource is used.
const testExternalAccountJSON = `{
    "type": "external_account",
    "audience": "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
    "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
    "token_url": "https://sts.googleapis.com/v1/token",
    "credential_source": {
        "file": "/var/run/secrets/tokens/gcp-token"
    }
}`

var _ = Describe("GetTokenSourceWithOptions", func() {
	ctx := context.Background()

	It("accepts a service-account JSON key by default", func() {
		ts, err := GetTokenSourceWithOptions(ctx, AuthOptions{
			Credential: testServiceAccountJSON,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(ts).ToNot(BeNil())
	})

	It("accepts an external_account JSON for workloadIdentityFederation", func() {
		ts, err := GetTokenSourceWithOptions(ctx, AuthOptions{
			CredentialType: CredentialTypeWorkloadIdentityFederation,
			Credential:     testExternalAccountJSON,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(ts).ToNot(BeNil())
	})

	It("rejects an external_account JSON when serviceAccountKey is required", func() {
		_, err := GetTokenSourceWithOptions(ctx, AuthOptions{
			CredentialType: CredentialTypeServiceAccountKey,
			Credential:     testExternalAccountJSON,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("service_account"))
	})

	It("rejects a service-account JSON when workloadIdentityFederation is required", func() {
		_, err := GetTokenSourceWithOptions(ctx, AuthOptions{
			CredentialType: CredentialTypeWorkloadIdentityFederation,
			Credential:     testServiceAccountJSON,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("external_account"))
	})

	It("rejects malformed JSON", func() {
		_, err := GetTokenSourceWithOptions(ctx, AuthOptions{
			Credential: "{not valid json",
		})
		Expect(err).To(HaveOccurred())
	})

	It("requires a credential for non-ADC modes", func() {
		_, err := GetTokenSourceWithOptions(ctx, AuthOptions{
			CredentialType: CredentialTypeServiceAccountKey,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("required"))
	})

	It("rejects an unknown credential type", func() {
		_, err := GetTokenSourceWithOptions(ctx, AuthOptions{
			CredentialType: "totally-not-a-real-mode",
			Credential:     testServiceAccountJSON,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported"))
	})

	Context("applicationDefault", func() {
		var (
			tmpDir          string
			origADCEnv      string
			origADCEnvWasOK bool
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "gke-adc-test-*")
			Expect(err).ToNot(HaveOccurred())
			path := filepath.Join(tmpDir, "adc.json")
			Expect(os.WriteFile(path, []byte(testServiceAccountJSON), 0o600)).To(Succeed())
			origADCEnv, origADCEnvWasOK = os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)).To(Succeed())
		})

		AfterEach(func() {
			if origADCEnvWasOK {
				Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADCEnv)).To(Succeed())
			} else {
				Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
			}
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		It("loads Application Default Credentials when no credential is provided", func() {
			ts, err := GetTokenSourceWithOptions(ctx, AuthOptions{
				CredentialType: CredentialTypeApplicationDefault,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(ts).ToNot(BeNil())
		})

		It("rejects a credential value when applicationDefault is selected", func() {
			_, err := GetTokenSourceWithOptions(ctx, AuthOptions{
				CredentialType: CredentialTypeApplicationDefault,
				Credential:     testServiceAccountJSON,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must not be provided"))
		})
	})
})
