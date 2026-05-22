package kasten

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

// collectSecurity inspects the authentication and encryption configuration.
func collectSecurity(c *cluster.Client, ns string) (SecurityConfig, error) {
	ctx := context.Background()
	sec := SecurityConfig{}

	// ── Auth detection ────────────────────────────────────────────
	// K10 stores OIDC/LDAP/auth config in the k10-config ConfigMap
	// and/or in Helm values secret.
	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "k10-config", metav1.GetOptions{})
	if err == nil {
		data := cm.Data

		switch {
		case data["authType"] == "OIDC" || data["oidcClientID"] != "" || data["oidcIssuerURL"] != "":
			sec.AuthMethod = "OIDC"
			sec.OIDCConfig = &OIDCInfo{
				ProviderURL:   data["oidcIssuerURL"],
				ClientID:      data["oidcClientID"],
				UsernameClaim: data["oidcUsernameClaim"],
				GroupsClaim:   data["oidcGroupClaim"],
			}

		case data["authType"] == "LDAP" || data["ldapHost"] != "":
			sec.AuthMethod = "LDAP"
			sec.LDAPConfig = &LDAPInfo{
				Host:       data["ldapHost"],
				BindDN:     data["ldapBindDN"],
				UserSearch: data["ldapUserSearch"],
				GroupSearch: data["ldapGroupSearch"],
			}

		case data["authType"] == "OpenShift" || data["openShiftOAuthEnabled"] == "true":
			sec.AuthMethod = "OpenShift OAuth"

		case data["authType"] == "Token" || data["tokenAuth"] == "true":
			sec.AuthMethod = "Token"

		case data["authType"] == "Basic" || data["basicAuth"] == "true":
			sec.AuthMethod = "Basic"

		default:
			sec.AuthMethod = "None / Passthrough"
		}
	}

	// Cross-check: look for OIDC/LDAP secrets
	if sec.AuthMethod == "" || sec.AuthMethod == "None / Passthrough" {
		secrets, _ := c.Typed.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
		if secrets != nil {
			for _, s := range secrets.Items {
				name := strings.ToLower(s.Name)
				switch {
				case strings.Contains(name, "oidc"):
					sec.AuthMethod = "OIDC"
				case strings.Contains(name, "ldap"):
					sec.AuthMethod = "LDAP"
				case strings.Contains(name, "openshift-oauth"):
					sec.AuthMethod = "OpenShift OAuth"
				}
			}
		}
	}

	if sec.AuthMethod == "" {
		sec.AuthMethod = "None / Passthrough"
	}

	// ── Encryption detection ──────────────────────────────────────
	sec.Encryption = detectEncryption(c, ns, ctx)

	return sec, nil
}

func detectEncryption(c *cluster.Client, ns string, ctx context.Context) EncryptionConfig {
	enc := EncryptionConfig{}

	// Check k10-config for encryption settings
	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "k10-config", metav1.GetOptions{})
	if err == nil {
		data := cm.Data

		switch {
		case data["awsKmsKeyId"] != "" || data["kmsKeyId"] != "":
			enc.Enabled = true
			enc.Provider = "AWS KMS"
			enc.KeyID = data["awsKmsKeyId"]
			if enc.KeyID == "" {
				enc.KeyID = data["kmsKeyId"]
			}

		case data["azureKeyVaultURI"] != "" || data["azureKeyVaultKeyName"] != "":
			enc.Enabled = true
			enc.Provider = "Azure Key Vault"
			enc.VaultURL = data["azureKeyVaultURI"]
			enc.KeyID = data["azureKeyVaultKeyName"]

		case data["vaultAddress"] != "" || data["vaultTransitPath"] != "":
			enc.Enabled = true
			enc.Provider = "HashiCorp Vault"
			enc.VaultURL = data["vaultAddress"]
			enc.KeyID = data["vaultTransitPath"]

		case data["encryptionEnabled"] == "true" || data["encryption"] == "true":
			enc.Enabled = true
			enc.Provider = "Kasten Managed"
		}
	}

	// Cross-check: look for KMS/Vault-related secrets and K10 Passkeys
	if !enc.Enabled {
		secrets, _ := c.Typed.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
		if secrets != nil {
			for _, s := range secrets.Items {
				name := strings.ToLower(s.Name)
				secType := string(s.Type)
				labels := s.Labels

				// K10 8.x Passkey: secrets labelled with k10.kasten.io/passkey
				// or of type containing "passkey" or "passphrase"
				if labels["k10.kasten.io/passkey"] != "" ||
					strings.Contains(secType, "passkey") ||
					strings.Contains(secType, "passphrase") ||
					(strings.Contains(name, "masterkey") && strings.Contains(secType, "kasten")) {
					enc.Enabled = true
					enc.Provider = "K10 Passphrase"
					enc.KeyID = s.Name
					return enc
				}

				switch {
				case strings.Contains(name, "kms") || strings.Contains(name, "aws-key"):
					enc.Enabled = true
					enc.Provider = "AWS KMS"
					return enc
				case strings.Contains(name, "keyvault") || strings.Contains(name, "azure-key"):
					enc.Enabled = true
					enc.Provider = "Azure Key Vault"
					return enc
				case strings.Contains(name, "vault-transit") || strings.Contains(name, "hashicorp"):
					enc.Enabled = true
					enc.Provider = "HashiCorp Vault"
					return enc
				}
			}
		}
	}

	return enc
}

// collectPasskeys finds K10 passkeys stored as Kubernetes secrets.
// K10 stores passkeys as secrets with specific labels or naming conventions.
func collectPasskeys(c *cluster.Client, ns string) []string {
	ctx := context.Background()
	var passkeys []string

	secrets, err := c.Typed.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return passkeys
	}

	for _, s := range secrets.Items {
		labels := s.Labels
		secType := string(s.Type)
		name := s.Name

		// K10 passkeys are identified by:
		// - label k10.kasten.io/passkey=true
		// - secret type containing "passkey" or "passphrase"
		// - name pattern like "k10*key" or "k10*passphrase"
		isPasskey := labels["k10.kasten.io/passkey"] == "true" ||
			strings.Contains(strings.ToLower(secType), "passkey") ||
			strings.Contains(strings.ToLower(secType), "passphrase") ||
			(strings.HasPrefix(strings.ToLower(name), "k10") &&
				(strings.Contains(strings.ToLower(name), "key") ||
					strings.Contains(strings.ToLower(name), "passphrase")))

		if isPasskey {
			passkeys = append(passkeys, name)
		}
	}
	return passkeys
}
