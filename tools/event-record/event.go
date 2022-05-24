package main

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/events"
	"time"
)

const PodName = "test-lifecycle-pod"
const ImageName = "busybox"
const ServiceName = "ServiceName"
const ContainerName = "busybox"
const DefaultNamespace = "default"

var terminationGracePeriod = int64(0)

func main() {
	logger := zap.NewExample().Sugar()

	config, err := clientcmd.BuildConfigFromFlags("", "/Users/yulongxin/.kube/config")
	if err != nil {
		logger.Panicw("create rest-config failed", "err", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Panicw("create rest client failed", "errr", err)
	}

	sinker := events.EventSinkImpl{Interface: client.EventsV1()}

	//新建一个eventbroadcast
	eventBroadcast := events.NewBroadcaster(&sinker)
	closeCh := make(chan struct{})
	// 开启eventboadcast，EventRecorder新建的信息会唤醒EventBroadcaster的go-routine，go-routine会消费信息，发送给EventSinker进而存储到kubernetes
	go eventBroadcast.StartRecordingToSink(closeCh)
	//新建一个record
	r := eventBroadcast.NewRecorder(scheme.Scheme, "test")
	pod, err := createPod(client)

	if err != nil {
		logger.Panicw("craete pod failed", "error", err)
	}
	logger.Infow("pod details", "name", pod.Name)
	//新建一个event，会到broadcaster的queue中，进而唤醒所有watch的go-routine，进而将信息发送到sinker中，存储到kubernetes
	r.Eventf(pod, pod, v1.EventTypeNormal, "test event record", "add", "message added %v", 1)
	r.Eventf(pod, pod, v1.EventTypeNormal, "test event record", "add", "message added %v", 2)

	//等到流程完成
	time.Sleep(time.Second)
	//关闭broadcster
	eventBroadcast.Shutdown()

}

func createPod(client *kubernetes.Clientset) (*v1.Pod, error) {
	pod, err := client.CoreV1().Pods(DefaultNamespace).Create(context.Background(), &v1.Pod{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   PodName,
			Labels: map[string]string{ServiceName: PodName},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    ContainerName,
					Image:   ImageName,
					Command: []string{"/bin/sh", "-c", "while :; do  date +%s ; sleep 1 ; done"},
					TTY:     true,
				},
			},
			TerminationGracePeriodSeconds: &terminationGracePeriod,
		},
	}, meta_v1.CreateOptions{})

	if err != nil && errors.IsAlreadyExists(err) {
		pod, err = client.CoreV1().Pods(DefaultNamespace).Get(context.Background(), PodName, meta_v1.GetOptions{})
		if err != nil {
			return nil, err
		}
		fmt.Println(pod.Name)
		return pod, nil
	}
	return pod, err
}
