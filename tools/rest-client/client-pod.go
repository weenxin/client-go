package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"net/http"
	"os"
	"time"
)

const PodName = "test-lifecycle-pod"
const ImageName = "busybox"
const ServiceName = "ServiceName"
const ContainerName = "busybox"
const DefaultNamespace = "default"

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", "/Users/yulongxin/.kube/config")
	if err != nil {
		panic(fmt.Sprintf("create client failed : %s", err))
	}
	client, err := kubernetes.NewForConfig(config)

	//create a pod
	terminationGracePeriod := int64(0)
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
	},
		meta_v1.CreateOptions{},
	)
	if err != nil {
		panic(fmt.Sprintf("can not create pod , err : %s", err))
	}

	waitForPodReady(client)
	fmt.Println("pod has been created")

	//程序退出时，删除pod
	defer client.CoreV1().Pods(DefaultNamespace).Delete(context.Background(), PodName, meta_v1.DeleteOptions{})

	pod, err = client.CoreV1().Pods(DefaultNamespace).Get(context.Background(), PodName, meta_v1.GetOptions{})
	if err != nil {
		panic(fmt.Sprintf("can not get pod , err : %s", err))
	}

	pod.Spec.Containers[0].Image = "busybox:1.35.0"
	pod, err = client.CoreV1().Pods(DefaultNamespace).Update(context.Background(), pod, meta_v1.UpdateOptions{})
	if err != nil {
		panic(fmt.Sprintf("can not upate pod, err : %s", err))
	}

	waitForPodReady(client)
	fmt.Println("pod has been updated")

	//测试listPod
	pods, err := client.CoreV1().Pods(DefaultNamespace).List(context.Background(), meta_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ServiceName: PodName}).String(),
	})
	fmt.Printf("pods count : %v \n", len(pods.Items))

	for index, pod := range pods.Items {
		fmt.Printf("pods[%d].name is %s \n", index, pod.Name)
	}

	logCloseCh := make(chan struct{})
	if len(pods.Items) > 0 {
		pod := pods.Items[0]
		sinceSeconds := int64(10)
		s, err := client.CoreV1().Pods(DefaultNamespace).GetLogs(pod.Name, &v1.PodLogOptions{SinceSeconds: &sinceSeconds, Follow: true}).Stream(context.Background())
		if err != nil {
			panic(fmt.Sprintf("get logs failed : %v", err))
		}
		b := bufio.NewScanner(s)
		go wait.Until(func() {
			if b.Scan() {
				fmt.Println("log line : ", b.Text())
			}
			if b.Err() != nil && b.Err() != io.EOF {
				panic(fmt.Sprintf("read from stream failed , error is : %v", b.Err()))
			}
		}, time.Millisecond*100, logCloseCh)

		time.Sleep(5 * time.Second)
		close(logCloseCh)
		time.Sleep(1 * time.Second)
		s.Close()
	}

	if len(pods.Items) > 0 {
		execCloseCh := make(chan struct{})

		req := client.CoreV1().RESTClient().Post().Namespace(DefaultNamespace).Resource("pods").Name(PodName).
			SubResource("exec").Param("container", pods.Items[0].Spec.Containers[0].Name).VersionedParams(&v1.PodExecOptions{
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
			Container: pods.Items[0].Name,
			Command:   []string{"/bin/sh"},
		}, scheme.ParameterCodec)

		exec, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, req.URL())
		if err != nil {
			panic(fmt.Sprintf("new spdy executor failed :%s", err))
		}

		go wait.Until(func() {
			err = exec.Stream(remotecommand.StreamOptions{
				Stdin:  os.Stdin,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
				Tty:    true,
			})
			//if err != nil {
			//		t.Fatalf("exec stream failed , err : %s", err)
			//	}
		}, time.Millisecond*100, execCloseCh)

		time.Sleep(10 * time.Second)
		close(execCloseCh)
		time.Sleep(time.Second)
	}

}

func waitForPodReady(client *kubernetes.Clientset) {
	//测试watcher
	watcher, err := client.CoreV1().Pods(DefaultNamespace).Watch(context.Background(), meta_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ServiceName: PodName}).String(),
	})
	if err != nil {
		panic(fmt.Sprintf("can not watch resource, err : %s", err))
	}

loop:
	for event := range watcher.ResultChan() {
		switch event.Type {
		case watch.Modified:
			thePod := event.Object.(*v1.Pod)
			for _, cond := range thePod.Status.Conditions {
				if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
					watcher.Stop()
					fmt.Println("pod status is ready")
					break loop
				}
			}
		case watch.Added:

		case watch.Bookmark:

		default:
			panic(fmt.Sprintf("unexcepted event type : %v, %v", event.Type, event.Object))
		}
	}
}
