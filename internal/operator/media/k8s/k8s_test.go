// K8s 媒介单测: fake clientset 验证操作能力 (client-go 标准测试姿势).
// 真实集群连通性测试需 TEST 环境凭据, 标注手测.
package k8s

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"JuanNiang-Ops/internal/operator/modules"
)

// newFakeClient 绕过 rest.Config 构造, 直接注入 fake clientset
func newFakeClient(cs *fake.Clientset) *Client {
	return &Client{cs: cs}
}

func TestListPods(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.1"},
			Spec:   corev1.PodSpec{NodeName: "node-a"}},
	)
	c := newFakeClient(cs)

	pods, err := c.ListPods(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 {
		t.Fatalf("pods = %d, want 1", len(pods))
	}
	if pods[0].Name != "web-1" || pods[0].Status != "Running" || pods[0].Node != "node-a" {
		t.Fatalf("pod info mismatch: %+v", pods[0])
	}
}

func TestGetDeployment(t *testing.T) {
	replicas := int32(3)
	cs := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "api"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "api:v2"}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2},
	})
	c := newFakeClient(cs)

	d, err := c.GetDeployment(context.Background(), "prod", "api")
	if err != nil {
		t.Fatal(err)
	}
	if d.Replicas != 3 || d.ReadyReplicas != 2 || len(d.Images) != 1 || d.Images[0] != "api:v2" {
		t.Fatalf("deployment info mismatch: %+v", d)
	}
}

func TestScaleDeployment(t *testing.T) {
	// fake 对 scale 子资源不做 Deployment<->Scale 自动转换 (已知缺口), 用 reactor 模拟:
	// GetScale 返回当前 Scale, UpdateScale 捕获写入的 Scale
	cs := fake.NewSimpleClientset()
	var scaled *autoscalingv1.Scale
	cs.PrependReactor("get", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "scale" {
			return true, &autoscalingv1.Scale{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", ResourceVersion: "1"},
				Spec:       autoscalingv1.ScaleSpec{Replicas: 1},
			}, nil
		}
		return false, nil, nil
	})
	cs.PrependReactor("update", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "scale" {
			scaled = action.(clienttesting.UpdateAction).GetObject().(*autoscalingv1.Scale)
			return true, scaled, nil
		}
		return false, nil, nil
	})
	c := newFakeClient(cs)

	if err := c.ScaleDeployment(context.Background(), "default", "api", 5); err != nil {
		t.Fatal(err)
	}
	if scaled == nil || scaled.Spec.Replicas != 5 {
		t.Fatalf("scaled = %+v, want replicas 5", scaled)
	}
}

func TestPodLogs(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-1"},
	})
	c := newFakeClient(cs)

	// fake clientset 的日志流返回字面量 "fake logs", 仅验证调用链路 (内容按真实集群手测)
	logs, err := c.PodLogs(context.Background(), "default", "web-1", modules.PodLogsOptions{TailLines: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs != "fake logs" {
		t.Fatalf("fake logs = %q, want %q", logs, "fake logs")
	}
}

func TestBuildRestConfigValidation(t *testing.T) {
	if _, err := buildRestConfig(modules.K8sConfig{}); err == nil || !strings.Contains(err.Error(), "empty access config") {
		t.Fatalf("want empty access config error, got %v", err)
	}
}
