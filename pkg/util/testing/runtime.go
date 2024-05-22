package testing

import (
	"os"
	"path/filepath"
	"time"

	"github.com/samber/lo"
)

func PrepareRunServerENV() string {
	ca := lo.Must(GenerateRootCertificate(2048, time.Hour))
	tmpPath := lo.Must(os.MkdirTemp("", ""))
	kubeconfig := lo.Must(ca.GenerateKubeconfig("https://127.0.0.1", "system:masters", "admin", time.Now().Add(time.Hour)))
	lo.Must0(os.WriteFile(filepath.Join(tmpPath, "kubeconfig"), kubeconfig, 0644))
	lo.Must0(lo.Must(ca.GenerateServerCerts("127.0.0.1", "", "", time.Now().Add(time.Hour))).IntoPath(tmpPath))
	return tmpPath
}
