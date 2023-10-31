package fpoc

import (
	"context"
	"crypto"

	"gitlab.com/gitlab-org/fleeting/fleeting/provider"
)

type PrivPub interface {
	crypto.PrivateKey
	Public() crypto.PublicKey
}

func (g *InstanceGroup) ssh(ctx context.Context, info *provider.ConnectInfo) error {
	/*

		var key PrivPub
		var err error

		if info.Key != nil {
			priv, err := ssh.ParseRawPrivateKey(info.Key)
			if err != nil {
				return fmt.Errorf("reading private key: %w", err)
			}
			var ok bool
			key, ok = priv.(PrivPub)
			if !ok {
				return fmt.Errorf("key doesn't export PublicKey()")
			}
		} else {
			key, err = rsa.GenerateKey(rand.Reader, 4096)
			if err != nil {
				return fmt.Errorf("generating private key: %w", err)
			}

			info.Key = pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
				},
			)
		}

		sshPubKey, err := ssh.NewPublicKey(key.Public())
		if err != nil {
			return fmt.Errorf("generating ssh public key: %w", err)
		}

	*/
	return nil
}
