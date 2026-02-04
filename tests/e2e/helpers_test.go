//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

const pollInterval = 5 * time.Second

// --- K8s helpers ---

// applyManifest decodes YAML bytes into an unstructured object and applies it
// via server-side apply. Usable from both *testing.T and TestMain.
func applyManifest(ctx context.Context, t *testing.T, yamlBytes []byte) {
	t.Helper()
	obj := &unstructured.Unstructured{}
	err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), len(yamlBytes)).Decode(obj)
	require.NoError(t, err, "decoding manifest YAML")

	obj.SetManagedFields(nil)
	err = k8sClient.Patch(ctx, obj, client.Apply, client.FieldOwner("e2e-test"), client.ForceOwnership)
	require.NoError(t, err, "server-side apply of %s/%s", obj.GetKind(), obj.GetName())
}

// mustApplyManifest is like applyManifest but for use in TestMain (no *testing.T).
func mustApplyManifest(ctx context.Context, yamlBytes []byte) {
	obj := &unstructured.Unstructured{}
	err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), len(yamlBytes)).Decode(obj)
	if err != nil {
		log.Fatalf("decoding manifest YAML: %v", err)
	}
	obj.SetManagedFields(nil)
	err = k8sClient.Patch(ctx, obj, client.Apply, client.FieldOwner("e2e-test"), client.ForceOwnership)
	if err != nil {
		log.Fatalf("server-side apply of %s/%s: %v", obj.GetKind(), obj.GetName(), err)
	}
}

// mustApplyCRDs runs kubectl apply on the CRD directory (for TestMain).
func mustApplyCRDs(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "deploy/charts/stratos/crds/")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("applying CRDs: %s: %v", string(out), err)
	}
}

// deleteIgnoreNotFound deletes a K8s object, ignoring NotFound errors.
func deleteIgnoreNotFound(ctx context.Context, obj client.Object) error {
	err := k8sClient.Delete(ctx, obj)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// --- Polling helpers ---

// waitForNodePoolStatus polls until the NodePool has the expected standby and
// running counts, or the timeout is reached.
func waitForNodePoolStatus(t *testing.T, ctx context.Context, name string, standby, running int32, timeout time.Duration) {
	t.Helper()
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		np, err := getNodePool(ctx, name)
		if !assert.NoError(ct, err, "fetching NodePool %s", name) {
			return
		}
		assert.Equal(ct, standby, np.Status.Standby, "standby count")
		assert.Equal(ct, running, np.Status.Running, "running count")
	}, timeout, pollInterval, "NodePool %s did not reach standby=%d running=%d", name, standby, running)
}

// waitForNodePoolStandbyGTE polls until standby >= minStandby.
func waitForNodePoolStandbyGTE(t *testing.T, ctx context.Context, name string, minStandby int32, timeout time.Duration) {
	t.Helper()
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		np, err := getNodePool(ctx, name)
		if !assert.NoError(ct, err) {
			return
		}
		assert.GreaterOrEqual(ct, np.Status.Standby, minStandby, "standby count")
	}, timeout, pollInterval, "NodePool %s standby did not reach >= %d", name, minStandby)
}

// waitForNoRunningNodes polls until there are 0 running nodes for the pool.
func waitForNoRunningNodes(t *testing.T, ctx context.Context, poolName string, timeout time.Duration) {
	t.Helper()
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		np, err := getNodePool(ctx, poolName)
		if !assert.NoError(ct, err) {
			return
		}
		assert.Equal(ct, int32(0), np.Status.Running, "running count")
	}, timeout, pollInterval, "NodePool %s still has running nodes", poolName)
}

// getNodePool fetches a NodePool by name (cluster-scoped).
func getNodePool(ctx context.Context, name string) (stratosv1alpha1.NodePool, error) {
	var np stratosv1alpha1.NodePool
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &np)
	return np, err
}

// --- AWS EC2 helpers ---

// listPoolInstances returns non-terminated EC2 instances for a pool/cluster.
func listPoolInstances(ctx context.Context, pool, cluster string) ([]ec2types.Instance, error) {
	out, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("tag:managed-by"), Values: []string{"stratos"}},
			{Name: aws.String("tag:stratos.sh/pool"), Values: []string{pool}},
			{Name: aws.String("tag:stratos.sh/cluster"), Values: []string{cluster}},
		},
	})
	if err != nil {
		return nil, err
	}
	var instances []ec2types.Instance
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			if inst.State != nil && inst.State.Name != ec2types.InstanceStateNameTerminated {
				instances = append(instances, inst)
			}
		}
	}
	return instances, nil
}

// countInstancesByEC2State returns the number of instances in a given EC2 nodestate.
func countInstancesByEC2State(ctx context.Context, pool, cluster string, state ec2types.InstanceStateName) (int, error) {
	instances, err := listPoolInstances(ctx, pool, cluster)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, inst := range instances {
		if inst.State != nil && inst.State.Name == state {
			count++
		}
	}
	return count, nil
}

// allInstancesInEC2State returns true if every instance is in the given nodestate.
func allInstancesInEC2State(ctx context.Context, pool, cluster string, state ec2types.InstanceStateName) (bool, error) {
	instances, err := listPoolInstances(ctx, pool, cluster)
	if err != nil {
		return false, err
	}
	if len(instances) == 0 {
		return false, nil
	}
	for _, inst := range instances {
		if inst.State == nil || inst.State.Name != state {
			return false, nil
		}
	}
	return true, nil
}

// instancesHaveType returns true if every instance matches the expected type.
func instancesHaveType(ctx context.Context, pool, cluster string, expectedType ec2types.InstanceType) (bool, error) {
	instances, err := listPoolInstances(ctx, pool, cluster)
	if err != nil {
		return false, err
	}
	if len(instances) == 0 {
		return false, nil
	}
	for _, inst := range instances {
		if inst.InstanceType != expectedType {
			return false, nil
		}
	}
	return true, nil
}

// terminatePoolInstances terminates all non-terminated instances for the pool.
func terminatePoolInstances(ctx context.Context, pool, cluster string) error {
	instances, err := listPoolInstances(ctx, pool, cluster)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return nil
	}
	var ids []string
	for _, inst := range instances {
		ids = append(ids, aws.ToString(inst.InstanceId))
	}
	_, err = ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: ids,
	})
	return err
}

// --- Node query helpers ---

// getNodesWithLabels lists K8s nodes matching the given label set.
func getNodesWithLabels(ctx context.Context, labels map[string]string) ([]corev1.Node, error) {
	var nodeList corev1.NodeList
	err := k8sClient.List(ctx, &nodeList, client.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// --- Cleanup ---

// cleanupAll performs idempotent full cleanup of E2E resources.
// Order: workloads -> NodePool (wait finalizer) -> AWSNodeClass (force finalizer)
//
//	-> K8s nodes -> EC2 instances (safety net).
//
// logf is used for logging (accepts log.Printf or t.Logf).
func cleanupAll(ctx context.Context, logf func(string, ...any)) {
	logf("cleanupAll: starting")

	// 1. Delete deployments + pod.
	for _, name := range []string{"e2e-scaleup-test", "e2e-precise-scaleup-test"} {
		dep := &appsv1.Deployment{}
		dep.Name = name
		dep.Namespace = "default"
		_ = deleteIgnoreNotFound(ctx, dep)
	}

	pod := &corev1.Pod{}
	pod.Name = "e2e-nomatch-test"
	pod.Namespace = "default"
	_ = deleteIgnoreNotFound(ctx, pod)

	// 2. Delete NodePools (force-remove finalizer immediately since no controller is running).
	for _, name := range []string{poolName} {
		np := &stratosv1alpha1.NodePool{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, np); err == nil {
			if len(np.Finalizers) > 0 {
				np.Finalizers = nil
				_ = k8sClient.Update(ctx, np)
			}
			_ = k8sClient.Delete(ctx, np)
		}
	}

	// 3. Delete AWSNodeClasses (force-remove finalizer immediately).
	for _, name := range []string{poolName} {
		nc := &stratosv1alpha1.AWSNodeClass{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, nc); err == nil {
			if len(nc.Finalizers) > 0 {
				nc.Finalizers = nil
				_ = k8sClient.Update(ctx, nc)
			}
			_ = k8sClient.Delete(ctx, nc)
		}
	}

	// 4. Delete stratos-labeled K8s nodes for both pools.
	for _, name := range []string{poolName} {
		nodeList := &corev1.NodeList{}
		if err := k8sClient.List(ctx, nodeList, client.MatchingLabels{nodestate.LabelPool: name}); err == nil {
			for i := range nodeList.Items {
				_ = k8sClient.Delete(ctx, &nodeList.Items[i])
			}
		}
	}

	// 5. Terminate EC2 instances via AWS API (safety net).
	for _, name := range []string{poolName} {
		if err := terminatePoolInstances(ctx, name, clusterName); err != nil {
			logf("cleanupAll: failed to terminate EC2 instances for pool %s: %v", name, err)
		}
	}

	logf("cleanupAll: done")
}

// logf adapters for cleanupAll.
func testLogf(t *testing.T) func(string, ...any) {
	return func(format string, args ...any) {
		t.Helper()
		t.Logf(format, args...)
	}
}

func stdLogf(format string, args ...any) {
	log.Print(fmt.Sprintf(format, args...))
}

// waitForHealthz polls the given URL until it returns 200 or the timeout expires.
func waitForHealthz(url string, timeout time.Duration) {
	c := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := c.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("controller healthz ready")
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Fatalf("controller healthz not ready after %s", timeout)
}
