package options_test

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/everoute/runtime/pkg/options"
	. "github.com/everoute/runtime/pkg/util/testing"
)

func TestNewRecommendedOptions(t *testing.T) {
	RegisterTestingT(t)

	tmpPath := PrepareRunServerENV()
	defer func() { Expect(os.RemoveAll(tmpPath)).ShouldNot(HaveOccurred()) }()

	opts := options.NewRecommendedOptions("/everoute/unittest", scheme.Codecs.LegacyCodec(metav1.SchemeGroupVersion))
	flagSet := pflag.NewFlagSet("", pflag.ContinueOnError)
	opts.AddFlags(flagSet)
	s := options.NewRecommendedConfig(scheme.Codecs)

	err := flagSet.Parse([]string{
		"-v=6",
		"--core-kubeconfig=" + filepath.Join(tmpPath, "kubeconfig"),
		"--serve-ca-crt-path=" + filepath.Join(tmpPath, "ca.crt"),
		"--serve-tls-crt-path=" + filepath.Join(tmpPath, "tls.crt"),
		"--serve-tls-key-path=" + filepath.Join(tmpPath, "tls.key"),
		"--contention-profiling",
		"--profiling=false",
		"--authentication-enabled",
		"--etcd-servers=http://127.0.0.1:2379",
		"--election-enabled",
	})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(opts.Validate()).Should(HaveLen(0))
	Expect(opts.ApplyTo(s)).ShouldNot(HaveOccurred())

	t.Run("should set klog options", func(t *testing.T) {
		Expect(klog.V(6).Enabled()).Should(BeTrue())
		Expect(klog.V(7).Enabled()).Should(BeFalse())
	})

	t.Run("should set core api options", func(t *testing.T) {
		Expect(s.ClientConfig).ShouldNot(BeNil())
		Expect(s.ClientConfig.Host).Should(Equal("https://127.0.0.1"))
		Expect(s.ClientConfig.RateLimiter).ShouldNot(BeNil())
		Expect(s.ClientConfig.RateLimiter.QPS()).Should(Equal(float32(1000)))
		Expect(s.Clientset).ShouldNot(BeNil())
	})

	t.Run("should set secure serving options", func(t *testing.T) {
		Expect(s.SecureServing).ShouldNot(BeNil())
		Expect(s.SecureServing.MinTLSVersion).Should(Equal(uint16(tls.VersionTLS12)))
		Expect(s.SecureServing.HTTP2MaxStreamsPerConnection).Should(Equal(1000))
	})

	t.Run("should set feature options", func(t *testing.T) {
		Expect(s.EnableProfiling).Should(BeFalse())
		Expect(s.EnableContentionProfiling).Should(BeTrue())
	})

	t.Run("should set authentication options", func(t *testing.T) {
		Expect(s.Authentication.Authenticator).ShouldNot(BeNil())
	})

	t.Run("should set etcd options", func(t *testing.T) {
		Expect(s.RESTOptionsGetter).ShouldNot(BeNil())
	})

	t.Run("should set election options", func(t *testing.T) {
		Expect(s.LeaderElectionClient).ShouldNot(BeNil())
		Expect(s.LeaderElectionClient.Identity()).ShouldNot(BeEmpty())
	})
}
