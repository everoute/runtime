package testing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	etcd3testing "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewTestingClient(t *testing.T, s *cobra.Command, args ...string) *rest.Config {
	RegisterTestingT(t)

	tmpPath := PrepareRunServerENV()
	t.Cleanup(func() { Expect(os.RemoveAll(tmpPath)).ShouldNot(HaveOccurred()) })
	server, etcdStorage := etcd3testing.NewUnsecuredEtcd3TestClientServer(t)
	t.Cleanup(func() { server.Terminate(t) })

	patch := gomonkey.ApplyFunc(storagebackend.NewDefaultConfig, func(prefix string, codec runtime.Codec) *storagebackend.Config {
		etcdStorage.Prefix = prefix
		etcdStorage.Codec = codec
		return etcdStorage
	})
	t.Cleanup(func() { patch.Reset() })

	randServerPort := rand.IntnRange(30000, 40000)
	os.Args = []string{
		"runtime-apiserver-unittest",
		"-v=6",
		"--serve-port=" + strconv.Itoa(randServerPort),
		"--core-kubeconfig=" + filepath.Join(tmpPath, "kubeconfig"),
		"--serve-ca-crt-path=" + filepath.Join(tmpPath, "ca.crt"),
		"--serve-tls-crt-path=" + filepath.Join(tmpPath, "tls.crt"),
		"--serve-tls-key-path=" + filepath.Join(tmpPath, "tls.key"),
		"--etcd-servers=" + strings.Join(server.V3Client.Endpoints(), ","),
	}
	if len(args) != 0 {
		os.Args = args
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	go func() { fmt.Println(s.ExecuteContext(ctx)) }()

	endpoint := fmt.Sprintf("https://127.0.0.1:%d/healthz", randServerPort)
	Eventually(func() bool { return IsServerHealth(endpoint, true) }, time.Minute, time.Second).Should(BeTrue())

	entrypoint := fmt.Sprintf("https://127.0.0.1:%d", randServerPort)
	kubeconfig := filepath.Join(tmpPath, "kubeconfig")
	return lo.Must(clientcmd.BuildConfigFromFlags(entrypoint, kubeconfig))
}
