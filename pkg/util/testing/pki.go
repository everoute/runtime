package testing

import (
	"crypto"
	"encoding/pem"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/lo"
	pkiutil "istio.io/istio/security/pkg/pki/util"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func GenerateRootCertificate(keySize int, ttl time.Duration) (*RootCertificate, error) {
	caContent, keyContent, err := pkiutil.GenCertKeyFromOptions(pkiutil.CertOptions{
		NotBefore:    time.Now().Add(-10 * time.Minute),
		TTL:          ttl,
		RSAKeySize:   keySize,
		IsCA:         true,
		IsSelfSigned: true,
	})
	if err != nil {
		return nil, err
	}
	return &RootCertificate{CAContent: caContent, CAKeyContent: keyContent}, nil
}

type RootCertificate struct {
	CAContent    []byte
	CAKeyContent []byte
}

func (c *RootCertificate) GenerateKubeconfig(server, organization, commonName string, expireTime time.Time) ([]byte, error) {
	crt, err := c.GenerateServerCerts("127.0.0.1", commonName, organization, expireTime)
	if err != nil {
		return nil, err
	}
	cluster := api.Cluster{Server: server, InsecureSkipTLSVerify: true}
	user := api.AuthInfo{ClientCertificateData: crt.CrtContent, ClientKeyData: crt.KeyContent}
	return clientcmd.Write(*newKubeConfig(&cluster, &user))
}

func (c *RootCertificate) GenerateServerCerts(ipSan, organization, commonName string, expireTime time.Time) (*TLSCertificate, error) {
	var csr, crt, key []byte
	var err error

	csr, key, err = pkiutil.GenCSR(pkiutil.CertOptions{
		NotBefore:  time.Now().Add(-10 * time.Minute),
		TTL:        -1 * time.Since(expireTime),
		Org:        organization,
		RSAKeySize: 2048,
		IsClient:   true,
		IsServer:   true,
		IsDualUse:  true,
		DNSNames:   commonName,
	})
	if err != nil {
		return nil, err
	}
	crt, err = pkiutil.GenCertFromCSR(
		lo.Must(pkiutil.ParsePemEncodedCSR(csr)),
		lo.Must(pkiutil.ParsePemEncodedCertificate(c.CAContent)),
		lo.Must(pkiutil.ParsePemEncodedKey(key)).(crypto.Signer).Public(),
		lo.Must(pkiutil.ParsePemEncodedKey(c.CAKeyContent)),
		[]string{ipSan},
		-1*time.Since(expireTime),
		false,
	)
	if err != nil {
		return nil, err
	}
	return &TLSCertificate{CAContent: c.CAContent, CrtContent: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: crt}), KeyContent: key}, nil
}

type TLSCertificate struct {
	CAContent  []byte
	CrtContent []byte
	KeyContent []byte
}

func (c *TLSCertificate) IntoPath(path string) error {
	err := os.WriteFile(filepath.Join(path, "ca.crt"), c.CAContent, 0600)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(path, "tls.crt"), c.CrtContent, 0600)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(path, "tls.key"), c.KeyContent, 0600)
	if err != nil {
		return err
	}
	return nil
}

func newKubeConfig(cluster *api.Cluster, authInfo *api.AuthInfo) *api.Config {
	const (
		clusterName = "cluster"
		userName    = "kubernetes-user"
		contextName = userName + "@" + clusterName
	)
	config := api.NewConfig()
	config.Clusters[clusterName] = cluster
	config.AuthInfos[userName] = authInfo
	config.Contexts[contextName] = &api.Context{
		Cluster:  clusterName,
		AuthInfo: userName,
	}
	config.CurrentContext = contextName
	return config
}
