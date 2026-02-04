//go:build e2e

package e2e

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// Package-level state shared across all tests.
var (
	k8sClient     client.Client
	ec2Client     *ec2.Client
	controllerCmd *exec.Cmd
	clusterName   string
	projectRoot   string
)

//go:embed testdata/nodeclass.yaml
var nodeClassYAML []byte

//go:embed testdata/nodepool.yaml
var nodePoolYAML []byte

//go:embed testdata/scaleup-deployment.yaml
var scaleUpDeploymentYAML []byte

//go:embed testdata/nomatch-pod.yaml
var noMatchPodYAML []byte

//go:embed testdata/scaleup-single-deployment.yaml
var preciseScaleUpDeploymentYAML []byte

const (
	poolName      = "al2023-basic"
	healthzPort   = 18081
)

func TestMain(m *testing.M) {
	// Resolve project root (tests/e2e -> project root).
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot = filepath.Join(filepath.Dir(thisFile), "..", "..")

	// 1. Cluster name from env.
	clusterName = os.Getenv("STRATOS_CLUSTER_NAME")
	if clusterName == "" {
		log.Fatal("STRATOS_CLUSTER_NAME must be set")
	}

	ctx := context.Background()

	// 2. Build K8s client.
	scheme := k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		log.Fatalf("adding client-go scheme: %v", err)
	}
	if err := stratosv1alpha1.AddToScheme(scheme); err != nil {
		log.Fatalf("adding stratos scheme: %v", err)
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("building kubeconfig: %v", err)
	}

	k8sClient, err = client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("creating k8s client: %v", err)
	}

	// 3. Build AWS EC2 client.
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("loading AWS config: %v", err)
	}
	ec2Client = ec2.NewFromConfig(awsCfg)

	// 4. Pre-cleanup.
	cleanupAll(ctx, stdLogf)

	// 5. Apply CRDs (safety net).
	mustApplyCRDs(ctx)

	// 6. Apply nodeclass + nodepool.
	mustApplyManifest(ctx, nodeClassYAML)
	mustApplyManifest(ctx, nodePoolYAML)

	// 7. Start controller subprocess.
	controllerCmd = exec.Command(
		"go", "run", "./cmd/stratos/main.go",
		"--cluster-name="+clusterName,
		"--cloud-provider=aws",
		"--metrics-bind-address=:0",
		fmt.Sprintf("--health-probe-bind-address=:%d", healthzPort),
	)
	controllerCmd.Dir = projectRoot
	controllerCmd.Stdout = os.Stdout
	controllerCmd.Stderr = os.Stderr
	// Use process group so we can signal the whole tree.
	controllerCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := controllerCmd.Start(); err != nil {
		log.Fatalf("starting controller: %v", err)
	}
	log.Printf("controller started (pid %d), waiting for healthz...", controllerCmd.Process.Pid)
	waitForHealthz(fmt.Sprintf("http://localhost:%d/healthz", healthzPort), 30*time.Second)

	// 8. Run tests.
	code := m.Run()

	// 9. Teardown.
	stopController()
	cleanupAll(ctx, stdLogf)

	os.Exit(code)
}

// stopController sends SIGINT, waits 15s, then SIGKILL.
func stopController() {
	if controllerCmd == nil || controllerCmd.Process == nil {
		return
	}
	log.Println("stopping controller...")
	// Signal the process group.
	_ = syscall.Kill(-controllerCmd.Process.Pid, syscall.SIGINT)

	done := make(chan error, 1)
	go func() { done <- controllerCmd.Wait() }()

	select {
	case <-done:
		log.Println("controller exited gracefully")
	case <-time.After(15 * time.Second):
		log.Println("controller did not exit in 15s, sending SIGKILL")
		_ = syscall.Kill(-controllerCmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
}

// ---------------------------------------------------------------------------
// Test Group 1: Pool Creation & Warmup
// ---------------------------------------------------------------------------

func TestPoolCreationAndWarmup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	t.Run("AWSNodeClass_resolves_selectors", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			var nc stratosv1alpha1.AWSNodeClass
			err := k8sClient.Get(ctx, types.NamespacedName{Name: poolName}, &nc)
			if !assert.NoError(ct, err) {
				return
			}
			assert.NotEmpty(ct, nc.Status.ResolvedAMI, "resolvedAMI")
			assert.NotEmpty(ct, nc.Status.ResolvedSubnets, "resolvedSubnets")
			assert.NotEmpty(ct, nc.Status.ResolvedSecurityGroups, "resolvedSecurityGroups")
			assert.NotEmpty(ct, nc.Status.ResolvedInstanceProfile, "resolvedInstanceProfile")
		}, 2*time.Minute, pollInterval, "AWSNodeClass selectors not resolved")
	})

	t.Run("nodes_reach_standby", func(t *testing.T) {
		waitForNodePoolStatus(t, ctx, poolName, 2, 0, 6*time.Minute)

		// Also verify warmup=0.
		np, err := getNodePool(ctx, poolName)
		require.NoError(t, err)
		require.Equal(t, int32(0), np.Status.Warmup, "warmup count should be 0")
	})

	t.Run("EC2_instances_stopped_t3medium", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			stopped, err := countInstancesByEC2State(ctx, poolName, clusterName, ec2types.InstanceStateNameStopped)
			if !assert.NoError(ct, err) {
				return
			}
			assert.Equal(ct, 2, stopped, "expected 2 stopped instances")
		}, 2*time.Minute, pollInterval, "EC2 instances not stopped in time")

		ok, err := instancesHaveType(ctx, poolName, clusterName, ec2types.InstanceTypeT3Medium)
		require.NoError(t, err)
		require.True(t, ok, "all instances should be t3.medium")
	})
}

// ---------------------------------------------------------------------------
// Test Group 2: Scale-Up
// ---------------------------------------------------------------------------

func TestScaleUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Apply the scale-up deployment.
	applyManifest(ctx, t, scaleUpDeploymentYAML)

	t.Run("pods_running_under_30s", func(t *testing.T) {
		start := time.Now()

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			var dep appsv1.Deployment
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-scaleup-test",
				Namespace: "default",
			}, &dep)
			if !assert.NoError(ct, err) {
				return
			}
			assert.Equal(ct, int32(2), dep.Status.ReadyReplicas, "ready replicas")
		}, 90*time.Second, pollInterval, "deployment pods not ready")

		duration := time.Since(start)
		t.Logf("scale-up duration: %s", duration)
	})

	t.Run("pods_on_pool_nodes_running_state", func(t *testing.T) {
		var podList corev1.PodList
		err := k8sClient.List(ctx, &podList, client.InNamespace("default"), client.MatchingLabels{"app": "e2e-scaleup"})
		require.NoError(t, err)
		require.Len(t, podList.Items, 2)

		for _, pod := range podList.Items {
			require.NotEmpty(t, pod.Spec.NodeName, "pod %s should be scheduled", pod.Name)

			var node corev1.Node
			err := k8sClient.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, &node)
			require.NoError(t, err, "fetching node %s", pod.Spec.NodeName)
			require.Equal(t, poolName, node.Labels[nodestate.LabelPool],
				"pod %s node should belong to pool", pod.Name)
			require.Equal(t, string(nodestate.NodeStateRunning), node.Labels[nodestate.LabelState],
				"pod %s node should be in running state", pod.Name)
		}
	})

	t.Run("nodepool_status_running1_standby1", func(t *testing.T) {
		np, err := getNodePool(ctx, poolName)
		require.NoError(t, err)
		require.Equal(t, int32(1), np.Status.Running, "running count")
		require.Equal(t, int32(1), np.Status.Standby, "standby count")
	})

	t.Run("nonmatching_selector_stays_pending", func(t *testing.T) {
		// Record pool total before.
		npBefore, err := getNodePool(ctx, poolName)
		require.NoError(t, err)
		totalBefore := npBefore.Status.Total

		// Apply the non-matching pod.
		applyManifest(ctx, t, noMatchPodYAML)

		// Wait for one full sync period (30s) + buffer so the controller has
		// had a chance to observe the pod and decide not to scale.
		time.Sleep(35 * time.Second)

		var pod corev1.Pod
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-nomatch-test", Namespace: "default"}, &pod)
		require.NoError(t, err)
		require.Equal(t, corev1.PodPending, pod.Status.Phase, "pod should stay Pending")

		// Verify pool total unchanged.
		npAfter, err := getNodePool(ctx, poolName)
		require.NoError(t, err)
		require.Equal(t, totalBefore, npAfter.Status.Total, "pool total should not change")

		// Cleanup.
		_ = deleteIgnoreNotFound(ctx, &pod)
	})
}

// ---------------------------------------------------------------------------
// Test Group 3: Scale-Down
// ---------------------------------------------------------------------------

func TestScaleDown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Delete the deployment to trigger scale-down.
	dep := &appsv1.Deployment{}
	dep.Name = "e2e-scaleup-test"
	dep.Namespace = "default"
	err := k8sClient.Delete(ctx, dep)
	require.NoError(t, err, "deleting deployment")

	t.Run("nodes_transition_to_standby", func(t *testing.T) {
		waitForNoRunningNodes(t, ctx, poolName, 4*time.Minute)
	})

	t.Run("nodepool_status_running0_standby_gte2", func(t *testing.T) {
		np, err := getNodePool(ctx, poolName)
		require.NoError(t, err)
		require.Equal(t, int32(0), np.Status.Running, "running count")
		require.GreaterOrEqual(t, np.Status.Standby, int32(2), "standby count")
	})

	t.Run("EC2_instances_stopped", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			allStopped, err := allInstancesInEC2State(ctx, poolName, clusterName, ec2types.InstanceStateNameStopped)
			if !assert.NoError(ct, err) {
				return
			}
			assert.True(ct, allStopped, "all instances should be stopped")

			runningCount, err := countInstancesByEC2State(ctx, poolName, clusterName, ec2types.InstanceStateNameRunning)
			if !assert.NoError(ct, err) {
				return
			}
			assert.Equal(ct, 0, runningCount, "no instances should be running")
		}, 3*time.Minute, pollInterval, "EC2 instances not all stopped")
	})
}

// ---------------------------------------------------------------------------
// Test Group 4: Precise Scale-Up (1 pod → exactly 1 node starts)
// ---------------------------------------------------------------------------

func TestPreciseScaleUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Precondition: no running nodes, at least 2 standby.
	np, err := getNodePool(ctx, poolName)
	require.NoError(t, err)
	require.Equal(t, int32(0), np.Status.Running, "precondition: running should be 0")
	standbyBefore := np.Status.Standby
	require.GreaterOrEqual(t, standbyBefore, int32(2), "precondition: need at least 2 standby")

	// Apply a single-replica deployment.
	applyManifest(ctx, t, preciseScaleUpDeploymentYAML)

	t.Run("exactly_1_node_starts", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			var dep appsv1.Deployment
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-precise-scaleup-test",
				Namespace: "default",
			}, &dep)
			if !assert.NoError(ct, err) {
				return
			}
			assert.Equal(ct, int32(1), dep.Status.ReadyReplicas, "ready replicas")
		}, 90*time.Second, pollInterval, "single pod not ready")

		np, err := getNodePool(ctx, poolName)
		require.NoError(t, err)
		require.Equal(t, int32(1), np.Status.Running, "should have exactly 1 running node")
		require.GreaterOrEqual(t, np.Status.Standby, int32(1), "should still have standby nodes")
	})

	t.Run("EC2_exactly_1_running", func(t *testing.T) {
		running, err := countInstancesByEC2State(ctx, poolName, clusterName, ec2types.InstanceStateNameRunning)
		require.NoError(t, err)
		require.Equal(t, 1, running, "exactly 1 EC2 instance should be running")

		stopped, err := countInstancesByEC2State(ctx, poolName, clusterName, ec2types.InstanceStateNameStopped)
		require.NoError(t, err)
		require.GreaterOrEqual(t, stopped, 1, "at least 1 EC2 instance should still be stopped")
	})

	// Cleanup: delete deployment, wait for scale-down so subsequent tests
	// (if any) start from a clean nodestate.
	t.Run("cleanup_scales_back_down", func(t *testing.T) {
		dep := &appsv1.Deployment{}
		dep.Name = "e2e-precise-scaleup-test"
		dep.Namespace = "default"
		err := k8sClient.Delete(ctx, dep)
		require.NoError(t, err)

		waitForNoRunningNodes(t, ctx, poolName, 4*time.Minute)
	})
}
