package benchmark

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	"k8s.io/klog/v2"
)

type Benchmark struct {
	clusterClient clientset.Interface

	recorder record.EventRecorder
}

var seq int

func New(clusterClient clientset.Interface, recorder record.EventRecorder) (*Benchmark, error) {

	b := &Benchmark{
		clusterClient: clusterClient,
		recorder:      recorder,
	}

	return b, nil
}

func (b *Benchmark) Run(stopChan <-chan struct{}) {
	go wait.Until(b.Step, 3*time.Minute, stopChan)
}

func (b *Benchmark) Step() {

	// List all pods in the kube-system namespace

	podList, err := b.clusterClient.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Infof("Fail to list Pods from kube-system with error %v", err)
		return
	}
	klog.Infof("Listing all pods in kube-system")
	for _, each := range podList.Items {
		klog.Infof("%s", each.Name)
	}
	seq++
	b.recorder.Eventf(&corev1.ObjectReference{
		Kind:      "Pod",
		Namespace: "kube-system",
		Name:      "vk-bench",
	}, corev1.EventTypeNormal, "Benchmark results", "there are %v pods in kube-system at iteration %v", len(podList.Items), seq)

}
