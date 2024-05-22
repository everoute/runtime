package options_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/rand"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/everoute/runtime/pkg/options"
	. "github.com/everoute/runtime/pkg/util/testing"
)

func TestNewElectionOptions(t *testing.T) {
	RegisterTestingT(t)

	tmpPath := PrepareRunServerENV()
	defer func() { Expect(os.RemoveAll(tmpPath)).ShouldNot(HaveOccurred()) }()

	opts := options.NewMultipleOptions(options.NewCoreAPIOptions(), options.NewElectionOptions())
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	opts.AddFlags(fs)
	s := options.NewRecommendedConfig(scheme.Codecs)

	var electionPostStartHookFunc genericapiserver.PostStartHookFunc
	patch := gomonkey.ApplyMethodFunc(&s.Config, "AddPostStartHook", func(name string, hook genericapiserver.PostStartHookFunc) error {
		electionPostStartHookFunc = hook
		return nil
	}).ApplyMethodReturn(&kubernetes.Clientset{}, "CoordinationV1", fake.NewSimpleClientset().CoordinationV1())
	defer patch.Reset()

	err := fs.Parse([]string{
		"--core-kubeconfig=" + filepath.Join(tmpPath, "kubeconfig"),
		"--election-enabled",
		"--election-identity=" + rand.String(20),
		"--election-name=" + rand.String(20),
	})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(opts.Validate()).Should(HaveLen(0))
	Expect(opts.ApplyTo(s)).ShouldNot(HaveOccurred())

	Expect(electionPostStartHookFunc).ShouldNot(BeNil())
	err = electionPostStartHookFunc(genericapiserver.PostStartHookContext{StopCh: make(chan struct{})})
	Expect(err).ShouldNot(HaveOccurred())

	ec := s.LeaderElectionClient
	Expect(ec).ShouldNot(BeNil())
	Expect(ec.Identity()).ShouldNot(BeEmpty())
	Eventually(ec.IsLeader).Should(BeTrue())
	Expect(ec.GetLeader()).Should(Equal(ec.Identity()))
}
