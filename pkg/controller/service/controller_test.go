package service_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("Service Reconcile", func() {
	var ctx context.Context
	var err error
	var cancel func()
	var service01, service02, service03 *corev1.Service

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(testTimeout))

		service01, err = createExternalService(ctx, serviceNamespaceName01)
		Expect(err).ShouldNot(HaveOccurred())
		service02, err = createExternalService(ctx, serviceNamespaceName02)
		Expect(err).ShouldNot(HaveOccurred())
		service03, err = createExternalService(ctx, types.NamespacedName{Namespace: rand.String(20), Name: rand.String(20)})
		Expect(err).ShouldNot(HaveOccurred())
	})
	AfterEach(func() {
		err := clientset.CoreV1().Services(service01.Namespace).Delete(ctx, service01.Name, metav1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		err = clientset.CoreV1().Services(service02.Namespace).Delete(ctx, service02.Name, metav1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		err = clientset.CoreV1().Services(service03.Namespace).Delete(ctx, service03.Name, metav1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		cancel()
	})

	When("election state is leading", func() {
		BeforeEach(func() {
			electionClient.SetLeader(name)
		})

		It("should update external name on include service", func() {
			Eventually(func() string {
				service, err := clientset.CoreV1().Services(service01.Namespace).Get(ctx, service01.Name, metav1.GetOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				return service.Spec.ExternalName
			}, testTimeout).Should(Equal(publicIP.String()))
		})

		It("should not update external name on exclude service", func() {
			time.Sleep(2 * time.Second) // wait for reconcile
			service, err := clientset.CoreV1().Services(service03.Namespace).Get(ctx, service03.Name, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(service.Spec.ExternalName).ShouldNot(Equal(publicIP.String()))
		})

		When("no-longer leading", func() {
			BeforeEach(func() {
				electionClient.SetLeader(rand.String(20))
			})

			It("should not update external name on include service", func() {
				Eventually(func() string {
					_, err := clientset.CoreV1().Services(service01.Namespace).Patch(ctx, service01.Name, types.JSONPatchType, []byte(`[{"op":"remove","path":"/spec/externalName"}]`), metav1.PatchOptions{})
					Expect(err).ShouldNot(HaveOccurred())
					time.Sleep(500 * time.Millisecond) // wait for reconcile
					service, err := clientset.CoreV1().Services(service01.Namespace).Get(ctx, service01.Name, metav1.GetOptions{})
					Expect(err).ShouldNot(HaveOccurred())
					return service.Spec.ExternalName
				}, testTimeout).ShouldNot(Equal(publicIP.String()))
			})
		})
	})

	When("election state is not leading", func() {
		BeforeEach(func() {
			electionClient.SetLeader(rand.String(20))
		})

		It("should not update external name on include service", func() {
			time.Sleep(2 * time.Second) // wait for reconcile
			service, err := clientset.CoreV1().Services(service02.Namespace).Get(ctx, service02.Name, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(service.Spec.ExternalName).ShouldNot(Equal(publicIP.String()))
		})

		When("election state is leading", func() {
			BeforeEach(func() {
				electionClient.SetLeader(name)
			})

			It("should update external name on include service", func() {
				Eventually(func() string {
					service, err := clientset.CoreV1().Services(service02.Namespace).Get(ctx, service02.Name, metav1.GetOptions{})
					Expect(err).ShouldNot(HaveOccurred())
					return service.Spec.ExternalName
				}, testTimeout).Should(Equal(publicIP.String()))
			})

			It("should not update external name on exclude service", func() {
				time.Sleep(2 * time.Second) // wait for reconcile
				service, err := clientset.CoreV1().Services(service03.Namespace).Get(ctx, service03.Name, metav1.GetOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(service.Spec.ExternalName).ShouldNot(Equal(publicIP.String()))
			})
		})
	})
})

func createExternalService(ctx context.Context, namespacedName types.NamespacedName) (*corev1.Service, error) {
	service := new(corev1.Service)
	service.SetNamespace(namespacedName.Namespace)
	service.SetName(namespacedName.Name)
	service.Spec.Type = corev1.ServiceTypeExternalName
	return clientset.CoreV1().Services(service.Namespace).Create(ctx, service, metav1.CreateOptions{})
}
