//go:build e2e

package e2e

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller"
)

//go:embed testdata/spot-nodeclass.yaml
var spotNodeClassYAML []byte

//go:embed testdata/spot-nodepool.yaml
var spotNodePoolYAML []byte

//go:embed testdata/spot-scaleup-deployment.yaml
var spotScaleUpDeploymentYAML []byte

const spotPoolName = "spot-basic"

// ---------------------------------------------------------------------------
// Test Group 5: Spot Pool Setup
// ---------------------------------------------------------------------------

func TestGroup5_SpotPoolSetup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	// Apply spot-specific manifests.
	applyManifest(ctx, t, spotNodeClassYAML)
	applyManifest(ctx, t, spotNodePoolYAML)

	t.Run("AWSNodeClass_resolves_selectors", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			var nc stratosv1alpha1.AWSNodeClass
			err := k8sClient.Get(ctx, types.NamespacedName{Name: spotPoolName}, &nc)
			if !assert.NoError(ct, err) {
				return
			}
			assert.NotEmpty(ct, nc.Status.ResolvedAMI, "resolvedAMI")
			assert.NotEmpty(ct, nc.Status.ResolvedSubnets, "resolvedSubnets")
			assert.NotEmpty(ct, nc.Status.ResolvedSecurityGroups, "resolvedSecurityGroups")
			assert.NotEmpty(ct, nc.Status.ResolvedInstanceProfile, "resolvedInstanceProfile")
		}, 2*time.Minute, pollInterval, "AWSNodeClass selectors not resolved for spot pool")
	})

	t.Run("OD_nodes_reach_standby", func(t *testing.T) {
		waitForNodePoolStandbyGTE(t, ctx, spotPoolName, 1, 6*time.Minute)

		np, err := getNodePool(ctx, spotPoolName)
		require.NoError(t, err)
		require.Equal(t, int32(0), np.Status.Warmup, "warmup count should be 0")
	})
}

// ---------------------------------------------------------------------------
// Test Group 6: Spot Replacement Lifecycle
// ---------------------------------------------------------------------------

func TestGroup6_SpotReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	// Apply deployment to trigger scale-up of an OD node.
	applyManifest(ctx, t, spotScaleUpDeploymentYAML)

	t.Run("OD_scaleup_pods_running", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			var dep appsv1.Deployment
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-spot-scaleup-test",
				Namespace: "default",
			}, &dep)
			if !assert.NoError(ct, err) {
				return
			}
			assert.Equal(ct, int32(1), dep.Status.ReadyReplicas, "ready replicas")
		}, 3*time.Minute, pollInterval, "spot deployment pods not ready")
	})

	// Track whether spot replacement was observed; later tests skip if not.
	spotReplacementObserved := false

	t.Run("spot_replacement_initiated", func(t *testing.T) {
		// Poll for the spot-replacement-started annotation on an OD node.
		// Use manual polling instead of require.EventuallyWithT so we can
		// skip (not fail) when spot replacement is unavailable.
		deadline := time.Now().Add(3 * time.Minute)
		for time.Now().Before(deadline) {
			nodes, err := getNodesWithLabels(ctx, map[string]string{
				controller.LabelPool:  spotPoolName,
				controller.LabelState: string(controller.NodeStateRunning),
			})
			if err == nil {
				for _, n := range nodes {
					if _, ok := n.Annotations[controller.AnnotationSpotReplacementStarted]; ok {
						spotReplacementObserved = true
						t.Logf("spot replacement initiated on node %s", n.Name)
						break
					}
				}
			}
			if spotReplacementObserved {
				break
			}
			time.Sleep(pollInterval)
		}
		if !spotReplacementObserved {
			t.Skip("Spot replacement not initiated — Spot capacity may be unavailable")
		}
	})

	t.Run("spot_node_running", func(t *testing.T) {
		if !spotReplacementObserved {
			t.Skip("Spot replacement was not initiated")
		}

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			nodes, err := getNodesWithLabels(ctx, map[string]string{
				controller.LabelPool:         spotPoolName,
				controller.LabelCapacityType: "spot",
				controller.LabelState:        string(controller.NodeStateRunning),
			})
			if !assert.NoError(ct, err) {
				return
			}
			assert.GreaterOrEqual(ct, len(nodes), 1, "expected at least 1 running spot node")
		}, 5*time.Minute, pollInterval, "spot node did not reach running state")
	})

	t.Run("OD_node_returns_to_standby", func(t *testing.T) {
		if !spotReplacementObserved {
			t.Skip("Spot replacement was not initiated")
		}

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			nodes, err := getNodesWithLabels(ctx, map[string]string{
				controller.LabelPool:         spotPoolName,
				controller.LabelCapacityType: "on-demand",
				controller.LabelState:        string(controller.NodeStateStandby),
			})
			if !assert.NoError(ct, err) {
				return
			}
			assert.GreaterOrEqual(ct, len(nodes), 1, "expected at least 1 OD node back in standby")
		}, 4*time.Minute, pollInterval, "OD node did not return to standby")
	})

	t.Run("workload_on_spot_node", func(t *testing.T) {
		if !spotReplacementObserved {
			t.Skip("Spot replacement was not initiated")
		}

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			var podList corev1.PodList
			err := k8sClient.List(ctx, &podList,
				client.InNamespace("default"),
				client.MatchingLabels{"app": "e2e-spot-scaleup"},
			)
			if !assert.NoError(ct, err) {
				return
			}
			if !assert.NotEmpty(ct, podList.Items, "no pods found") {
				return
			}

			for _, pod := range podList.Items {
				if pod.Spec.NodeName == "" {
					assert.Fail(ct, "pod not scheduled", "pod %s has no nodeName", pod.Name)
					continue
				}
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, &node)
				if !assert.NoError(ct, err, "fetching node %s", pod.Spec.NodeName) {
					continue
				}
				assert.Equal(ct, "spot", node.Labels[controller.LabelCapacityType],
					"pod %s should be on a spot node", pod.Name)
			}
		}, 3*time.Minute, pollInterval, "workload not on spot node")
	})

	t.Run("EC2_spot_instance_running", func(t *testing.T) {
		if !spotReplacementObserved {
			t.Skip("Spot replacement was not initiated")
		}

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			spotInstances, err := listPoolInstancesByLifecycle(ctx, spotPoolName, clusterName, true)
			if !assert.NoError(ct, err) {
				return
			}
			runningSpot := 0
			for _, inst := range spotInstances {
				if inst.State != nil && inst.State.Name == ec2types.InstanceStateNameRunning {
					runningSpot++
				}
			}
			assert.GreaterOrEqual(ct, runningSpot, 1, "expected at least 1 running EC2 spot instance")
		}, 30*time.Second, pollInterval, "no running EC2 spot instance found")
	})
}

// ---------------------------------------------------------------------------
// Test Group 7: Spot Interruption Fallback
// ---------------------------------------------------------------------------

func TestGroup7_SpotInterruption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	// Precondition: skip if no spot node exists (Group 6 was skipped).
	spotNodes, err := getNodesWithLabels(ctx, map[string]string{
		controller.LabelPool:         spotPoolName,
		controller.LabelCapacityType: "spot",
	})
	if err != nil || len(spotNodes) == 0 {
		t.Skip("No spot nodes exist — skipping interruption tests")
	}

	t.Run("terminate_spot_instances", func(t *testing.T) {
		spotInstances, err := listPoolInstancesByLifecycle(ctx, spotPoolName, clusterName, true)
		require.NoError(t, err)
		require.NotEmpty(t, spotInstances, "no spot EC2 instances found")

		// Terminate all running spot instances to simulate interruption.
		var targetIDs []string
		for _, inst := range spotInstances {
			if inst.State != nil && inst.State.Name == ec2types.InstanceStateNameRunning {
				targetIDs = append(targetIDs, aws.ToString(inst.InstanceId))
			}
		}
		require.NotEmpty(t, targetIDs, "no running spot instances to terminate")

		t.Logf("terminating %d spot instances: %v", len(targetIDs), targetIDs)
		_, err = ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: targetIDs,
		})
		require.NoError(t, err, "terminating spot instances")
	})

	t.Run("spot_node_cleaned_up", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			nodes, err := getNodesWithLabels(ctx, map[string]string{
				controller.LabelPool:         spotPoolName,
				controller.LabelCapacityType: "spot",
			})
			if !assert.NoError(ct, err) {
				return
			}
			assert.Empty(ct, nodes, "spot K8s nodes should be cleaned up")
		}, 2*time.Minute, pollInterval, "spot node not cleaned up after termination")
	})

	t.Run("OD_fallback_pods_running", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			var dep appsv1.Deployment
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-spot-scaleup-test",
				Namespace: "default",
			}, &dep)
			if !assert.NoError(ct, err) {
				return
			}
			assert.Equal(ct, int32(1), dep.Status.ReadyReplicas, "deployment should have 1 ready replica")

			// Verify pod is on an OD node (all spot nodes were terminated).
			var podList corev1.PodList
			listErr := k8sClient.List(ctx, &podList,
				client.InNamespace("default"),
				client.MatchingLabels{"app": "e2e-spot-scaleup"},
			)
			if !assert.NoError(ct, listErr) {
				return
			}
			for _, pod := range podList.Items {
				if pod.Spec.NodeName == "" {
					continue
				}
				var node corev1.Node
				nodeErr := k8sClient.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, &node)
				if !assert.NoError(ct, nodeErr, "fetching node %s", pod.Spec.NodeName) {
					continue
				}
				assert.Equal(ct, "on-demand", node.Labels[controller.LabelCapacityType],
					"pod %s should be on an OD node after spot interruption", pod.Name)
			}
		}, 3*time.Minute, pollInterval, "deployment pods not ready on OD node after spot interruption")
	})
}

// ---------------------------------------------------------------------------
// Test Group 8: Spot Cleanup
// ---------------------------------------------------------------------------

func TestGroup8_SpotCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Delete the deployment first.
	dep := &appsv1.Deployment{}
	dep.Name = "e2e-spot-scaleup-test"
	dep.Namespace = "default"
	_ = deleteIgnoreNotFound(ctx, dep)

	t.Run("no_running_nodes_in_spot_pool", func(t *testing.T) {
		waitForNoRunningNodes(t, ctx, spotPoolName, 3*time.Minute)
	})

	t.Run("cleanup_spot_resources", func(t *testing.T) {
		cleanupSpotPool(ctx, testLogf(t))
	})
}
