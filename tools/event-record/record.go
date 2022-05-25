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
	typedv1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
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

	//新建sinker
	sinker := typedv1core.EventSinkImpl{Interface: client.CoreV1().Events("")}

	// 创建修改器，event最多1条，模拟合并操作
	option := record.CorrelatorOptions{
		MaxEvents: 2,
	}
	//创建broadcaster
	broadcaster := record.NewBroadcasterWithCorrelatorOptions(option)

	//写到APISserver
	go broadcaster.StartRecordingToSink(&sinker)
	// 写到日志
	go broadcaster.StartLogging(logger.Infof)
	// 事件监听
	go broadcaster.StartEventWatcher(func(event *v1.Event) {
		logger.Infow("watch message ", "event", event)
	})

	// 增加一个Recorder
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{
		Component: "test",
		Host:      "weenxin",
	})

	// 创建一个pod
	pod, err := createOrGetPod(client)

	if err != nil {
		logger.Panicw("craete pod failed", "error", err)
	}
	logger.Infow("pod details", "name", pod.Name)
	//新建一个event，会到broadcaster的queue中，进而唤醒所有watch的go-routine，进而将信息发送到sinker中，存储到kubernetes
	recorder.Eventf(pod, v1.EventTypeNormal, "test event record", "add", "message added %v", 1)
	//新建一个event，会到broadcaster的queue中，进而唤醒所有watch的go-routine，进而将信息发送到sinker中，存储到kubernetes
	recorder.Eventf(pod, v1.EventTypeNormal, "test event record", "add", "message added %v", 2)

	//等到流程完成
	time.Sleep(time.Second)
	//关闭broadcster
	broadcaster.Shutdown()

}

func createOrGetPod(client *kubernetes.Clientset) (*v1.Pod, error) {
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
