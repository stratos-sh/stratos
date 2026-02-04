//go:build integration
// +build integration

/*
Copyright 2026 Stratos Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	awsprovider "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
	"github.com/stratos-sh/stratos/internal/cloudprovider/fake"
	"github.com/stratos-sh/stratos/internal/controller/nodeclass"
	"github.com/stratos-sh/stratos/internal/controller/nodepool"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// Test suite variables
var (
	cfg         *rest.Config
	k8sClient   client.Client
	testEnv     *envtest.Environment
	ctx         context.Context
	cancel      context.CancelFunc
	testScheme  *runtime.Scheme
	fakeProvider *fake.FakeProvider
	fakeResolver *fake.FakeResolver
	reconciler  *nodepool.Reconciler
	mgr         ctrl.Manager
)

const (
	testClusterName = "test-cluster"
	timeout         = 30 * time.Second
	interval        = 100 * time.Millisecond
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "deploy", "charts", "stratos", "crds")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Set up scheme
	testScheme = runtime.NewScheme()
	Expect(scheme.AddToScheme(testScheme)).To(Succeed())
	Expect(stratosv1alpha1.AddToScheme(testScheme)).To(Succeed())
	Expect(corev1.AddToScheme(testScheme)).To(Succeed())

	// Create client
	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create manager
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
	})
	Expect(err).NotTo(HaveOccurred())

	// Create fake provider
	fakeProvider = fake.NewFakeProvider()

	// Create and register the reconciler
	reconciler = &nodepool.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		ClusterName:   testClusterName,
		CloudProvider: "fake",
	}
	reconciler.InjectCloudProvider("default", fakeProvider)

	err = reconciler.SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	// Register the NodeClass lifecycle controller
	err = nodeclass.Setup(mgr)
	Expect(err).NotTo(HaveOccurred())

	// Create and register the AWSNodeClass reconciler with FakeResolver
	fakeResolver = fake.NewFakeResolver()
	awsNodeClassReconciler := &awsprovider.AWSNodeClassReconciler{
		Client:            mgr.GetClient(),
		Resolver:          fakeResolver,
		ClusterName:       testClusterName,
		KubernetesVersion: "1.34", // Fixed version for testing AMI auto-discovery
	}
	err = awsNodeClassReconciler.SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	// Start manager in background
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = BeforeEach(func() {
	// Reset the fake provider before each test
	fakeProvider.Reset()
	fakeProvider.LaunchHook = nil
	fakeProvider.StartHook = nil
	fakeProvider.StopHook = nil
	fakeProvider.TerminateHook = nil

	// Reset fake resolver hooks
	fakeResolver.ResolveAMIHook = nil
	fakeResolver.ResolveSubnetsHook = nil
	fakeResolver.ResolveSecurityGroupsHook = nil
	fakeResolver.ResolveInstanceProfileHook = nil
	fakeResolver.DeleteInstanceProfileHook = nil
})

var _ = AfterEach(func() {
	// Clean up any NodePools and Nodes created during the test
	cleanupTestResources()
})

// cleanupTestResources removes all test resources after each test
func cleanupTestResources() {
	// Delete all Pods in default and test namespaces to prevent test isolation issues
	// (pending pods from previous tests can trigger scale-up in subsequent tests)
	podList := &corev1.PodList{}
	err := k8sClient.List(ctx, podList)
	if err == nil {
		for i := range podList.Items {
			pod := &podList.Items[i]
			// Delete test pods (skip system pods in kube-system)
			if pod.Namespace != "kube-system" {
				_ = k8sClient.Delete(ctx, pod)
			}
		}
	}

	// Force-delete NodePools with retry loop to win the race against reconciler re-adding finalizers
	Eventually(func() int {
		list := &stratosv1alpha1.NodePoolList{}
		_ = k8sClient.List(ctx, list)
		for i := range list.Items {
			np := &list.Items[i]
			// Re-fetch to get latest resourceVersion (reconciler may have re-added finalizer)
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(np), np)
			if len(np.Finalizers) > 0 {
				np.Finalizers = nil
				_ = k8sClient.Update(ctx, np)
			}
			_ = k8sClient.Delete(ctx, np)
		}
		return len(list.Items)
	}, timeout, interval).Should(Equal(0))

	// Force-delete AWSNodeClasses with same retry pattern
	Eventually(func() int {
		list := &stratosv1alpha1.AWSNodeClassList{}
		_ = k8sClient.List(ctx, list)
		for i := range list.Items {
			nc := &list.Items[i]
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(nc), nc)
			if len(nc.Finalizers) > 0 {
				nc.Finalizers = nil
				_ = k8sClient.Update(ctx, nc)
			}
			_ = k8sClient.Delete(ctx, nc)
		}
		return len(list.Items)
	}, timeout, interval).Should(Equal(0))

	// Delete all Nodes with stratos labels
	nodeList := &corev1.NodeList{}
	err = k8sClient.List(ctx, nodeList)
	if err == nil {
		for i := range nodeList.Items {
			node := &nodeList.Items[i]
			if _, ok := node.Labels[nodestate.LabelPool]; ok {
				_ = k8sClient.Delete(ctx, node)
			}
		}
	}
}
