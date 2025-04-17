package fpoc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/hashicorp/go-hclog"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/fleeting/fleeting/provider"
)

type PrivPub interface {
	crypto.PrivateKey
	Public() crypto.PublicKey
}

// initSSHKey prepare dynamic ssh key for flatcar instances
func (g *InstanceGroup) initSSHKey(_ context.Context, log hclog.Logger, settings *provider.Settings) error {
	var key PrivPub
	var err error

	if len(settings.Key) == 0 {
		log.Info("Generating dynamic SSH key...")

		key, err = rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return fmt.Errorf("generating private key: %w", err)
		}
		settings.Key = pem.EncodeToMemory(
			&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
			},
		)

		log.Debug("Key generated")
	} else {
		var ok bool

		priv, err := ssh.ParseRawPrivateKey(settings.Key)
		if err != nil {
			return fmt.Errorf("reading private key: %w", err)
		}

		key, ok = priv.(PrivPub)
		if !ok {
			return fmt.Errorf("key doesn't export PublicKey()")
		}
	}

	log.Debug("Extracting public key...")
	sshPubKey, err := ssh.NewPublicKey(key.Public())
	if err != nil {
		return fmt.Errorf("generating private key: %w", err)
	}

	g.sshPubKey = string(ssh.MarshalAuthorizedKey(sshPubKey))
	log.With("public_key", g.sshPubKey).Debug("Extracted public key")

	imgProps := g.imgProps.Load()
	if imgProps != nil {
		if imgProps.OSAdminUser == "" && settings.Username == "" {
			// nolint:staticcheck
			return fmt.Errorf("image properties 'os_admin_user' and 'runners.autoscaler.connector_config.username' missing. Ensure one is set.")
		}
		if imgProps.OSAdminUser != "" && settings.Username == "" {
			settings.Username = imgProps.OSAdminUser
		}
	}

	return nil
}
