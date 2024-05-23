package service_test

import (
	"net"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/everoute/runtime/pkg/controller/service"
	. "github.com/everoute/runtime/pkg/util/testing"
)

var (
	clientset      kubernetes.Interface
	f              informers.SharedInformerFactory
	name           string
	electionClient *FakeLeaderElectionClient
	publicIP       net.IP

	testTimeout            = 10
	serviceNamespaceName01 = types.NamespacedName{Namespace: rand.String(20), Name: rand.String(20)}
	serviceNamespaceName02 = types.NamespacedName{Namespace: rand.String(20), Name: rand.String(20)}
	stopCh                 = make(chan struct{})
)

func TestServiceReconcile(t *testing.T) {
	RegisterTestingT(t)
	RunSpecs(t, "Service Reconcile Test Suite")
}

var _ = BeforeSuite(func() {
	clientset = fake.NewSimpleClientset()
	f = informers.NewSharedInformerFactory(clientset, 0)
	name = rand.String(20)
	electionClient = NewFakeLeaderElectionClient(name)
	publicIP = net.ParseIP("10.1.0.1")

	serviceController := service.New(clientset, f, electionClient, publicIP, 0,
		serviceNamespaceName01.String(),
		serviceNamespaceName02.String(),
	)

	go serviceController.Run(stopCh)

	f.Start(stopCh)
	f.WaitForCacheSync(stopCh)
})

var _ = AfterSuite(func() { close(stopCh) })
