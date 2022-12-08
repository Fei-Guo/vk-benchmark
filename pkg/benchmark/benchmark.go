package benchmark

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	"k8s.io/klog/v2"
)

const (
	virtualNodeName    = "virtual-node-aci-linux"
	benchmarkNamespace = "vk-bench"

	numBackgroundPods = 2
	numPeriodicPods   = 2

	pollInterval = time.Second * 10
	pollTimeout  = time.Second * 120

	patrolInterval = time.Second * 600
)

type Benchmark struct {
	clusterClient clientset.Interface

	recorder record.EventRecorder
}

func testPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "default",
			Containers: []corev1.Container{
				{
					Name:    "aci-sample",
					Image:   "busybox",
					Command: []string{"top"},
				},
			},
			NodeSelector: map[string]string{
				"kubernetes.io/role":    "agent",
				"beta.kubernetes.io/os": "linux",
				"type":                  "virtual-kubelet",
			},
			Tolerations: []corev1.Toleration{
				corev1.Toleration{
					Key:      "virtual-kubelet.io/provider",
					Operator: corev1.TolerationOpExists,
				},
			},
		},
	}
}

func testNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func applyNodeNameToPod(vPod *corev1.Pod, nodeName string) *corev1.Pod {
	vPod.Spec.NodeName = nodeName
	return vPod
}

func New(clusterClient clientset.Interface, recorder record.EventRecorder) (*Benchmark, error) {
	b := &Benchmark{
		clusterClient: clusterClient,
		recorder:      recorder,
	}
	return b, nil
}

func (b *Benchmark) Run(stopChan <-chan struct{}) {

	if err := b.StartBackgroundPods(); err != nil {
		klog.Infof("Fail to create background pods with error %v", err)
		return
	}

	go wait.Until(b.Patroll, patrolInterval, stopChan)
}

func (b *Benchmark) StartBackgroundPods() error {
	_, err := b.CheckVnodeStatus()
	if err != nil {
		return err
	}

	// First create a namespace
	_, cerr := b.clusterClient.CoreV1().Namespaces().Create(context.TODO(), testNamespace(benchmarkNamespace), metav1.CreateOptions{})
	if cerr != nil && !apierrors.IsAlreadyExists(cerr) {
		return cerr
	}

	// Create background pods
	for i := 0; i < numBackgroundPods; i++ {
		p := testPod(fmt.Sprintf("background-%d", i), benchmarkNamespace)
		_, cerr := b.clusterClient.CoreV1().Pods(benchmarkNamespace).Create(context.TODO(), p, metav1.CreateOptions{})
		if cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return cerr
		}
	}
	return nil
}

func (b *Benchmark) CreatePeriodicPods() int {
	newPods := []string{}
	for i := 0; i < numPeriodicPods; i++ {
		p := testPod(fmt.Sprintf("periodic-%d", i), benchmarkNamespace)
		_, cerr := b.clusterClient.CoreV1().Pods(benchmarkNamespace).Create(context.TODO(), p, metav1.CreateOptions{})
		if cerr != nil {
			if apierrors.IsAlreadyExists(cerr) {
				klog.Infof("Periodic Pod %s still exists, it is not deleted in previous iteration!", p.Name)
			} else {
				klog.Infof("Fail to create periodic Pod %s with error %v", p.Name, cerr)
			}
		} else {
			newPods = append(newPods, p.Name)
		}
	}

	// Wait until new Pods are running
	ret := 0
	for _, each := range newPods {
		if err := b.waitForPodRunning(each, benchmarkNamespace, pollTimeout); err != nil {
			klog.Infof("Fail to wait periodic Pod %s running with error %v", each, err)
		} else {
			ret++
		}
	}
	return ret
}

func (b *Benchmark) DeletePeriodicPods() int {
	deletedPods := []string{}
	for i := 0; i < numPeriodicPods; i++ {
		name := fmt.Sprintf("periodic-%d", i)
		_, gerr := b.clusterClient.CoreV1().Pods(benchmarkNamespace).Get(context.TODO(), name, metav1.GetOptions{})
		if gerr != nil {
			if apierrors.IsNotFound(gerr) {
				klog.Infof("Periodic Pod %s has gone unexpectedly!", name)
			} else {
				klog.Infof("Fail to get periodic Pod %s before deletion with error %v", name, gerr)
			}
		} else {
			if derr := b.clusterClient.CoreV1().Pods(benchmarkNamespace).Delete(context.TODO(), name, metav1.DeleteOptions{}); derr != nil {
				klog.Infof("Fail to delete periodic Pod %s with error %v", name, derr)
			} else {
				deletedPods = append(deletedPods, name)
			}
		}
	}

	// Wait until deleted Pods are gone
	ret := 0
	for _, each := range deletedPods {
		if err := b.waitForPodDeleted(each, benchmarkNamespace, pollTimeout); err != nil {
			klog.Infof("Fail to wait periodic Pod %s deleted with error %v", each, err)
		} else {
			ret++
		}
	}
	return ret
}

func (b *Benchmark) CheckBackgroundPods() int {
	ret := 0
	for i := 0; i < numBackgroundPods; i++ {
		name := fmt.Sprintf("background-%d", i)
		if err := b.waitForPodRunning(name, benchmarkNamespace, pollTimeout); err != nil {
			klog.Infof("Fail to wait background Pod %s running with error %v", name, err)
		} else {
			ret++
		}
	}
	return ret
}

func (b *Benchmark) Patroll() {
	var numBackgroundPods, numDeletedPods, numCreatedPods int
	var vkVersion string
	var err error

	// Check node status in every iteration in case vk image is updated or node is offline
	vkVersion, err = b.CheckVnodeStatus()
	if err == nil {
		numBackgroundPods = b.CheckBackgroundPods()
		numCreatedPods = b.CreatePeriodicPods()
		numDeletedPods = b.DeletePeriodicPods()
	} else {
		vkVersion = "unknown"
	}

	b.recorder.Eventf(&corev1.ObjectReference{
		Kind:      "Pod",
		Namespace: "kube-system",
		Name:      "vk-bench",
	}, corev1.EventTypeNormal, "Benchmark results", "Backgroud Ready Pods %d, Periodic Delete Pods %d, Periodic Create Pods %d, vk version %s",
		numBackgroundPods, numDeletedPods, numCreatedPods, vkVersion)

}

func (b *Benchmark) CheckVnodeStatus() (string, error) {
	nodeList, err := b.clusterClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Infof("Fail to list Nodes from kube-system with error %v", err)
		return "", err
	}
	for _, each := range nodeList.Items {
		for _, c := range each.Status.Conditions {
			if each.Name == virtualNodeName && c.Reason == "KubeletReady" && c.Status == v1.ConditionTrue && c.Type == v1.NodeReady {
				return each.Status.NodeInfo.KubeletVersion, nil
			}
		}
	}
	return "", fmt.Errorf("Fail to find a running virtual node that is in the READY state")
}

func (b *Benchmark) isPodRunning(name, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		fmt.Printf(".") // progress bar!

		pod, err := b.clusterClient.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		switch pod.Status.Phase {
		case v1.PodRunning:
			return true, nil
		case v1.PodFailed, v1.PodSucceeded:
			return false, fmt.Errorf("pod is failed/completed state")
		}
		return false, nil
	}
}

func (b *Benchmark) waitForPodRunning(name, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(pollInterval, timeout, b.isPodRunning(name, namespace))
}

func (b *Benchmark) isPodDeleted(name, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		fmt.Printf(".") // progress bar!

		_, err := b.clusterClient.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			} else {
				return false, err
			}
		}
		return false, nil
	}
}

func (b *Benchmark) waitForPodDeleted(name, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(pollInterval, timeout, b.isPodDeleted(name, namespace))
}
