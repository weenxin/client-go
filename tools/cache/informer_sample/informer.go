package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ActionHandler struct{
}

func (handler *ActionHandler) OnAdd(obj interface{}) {
	pod , ok  := obj.(*v1.Pod)
	if !ok {
		panic("wrong types")
	}
	fmt.Printf("object added : %s \n", pod.ObjectMeta.String() )
}
func (handler *ActionHandler) OnUpdate(oldObj, newObj interface{}) {
	pod , ok  := newObj.(*v1.Pod)
	if !ok {
		panic("wrong types")
	}
	fmt.Printf("object updated : %s \n ", pod.ObjectMeta.String() )
}

func (handler *ActionHandler) OnDelete(obj interface{}) {
	pod , ok  := obj.(*v1.Pod)
	if !ok {
		panic("wrong types")
	}
	fmt.Printf("object deleted : %s \n ", pod.ObjectMeta.String() )
}




func main() {
	// creates the connection
	config, err := clientcmd.BuildConfigFromFlags("", "/Users/weenxin/.kube/config")
	if err != nil {
		klog.Fatal(err)
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err)
	}

	// create the pod watcher
	lw := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "pods", v1.NamespaceDefault, fields.Everything())

	_, informer := cache.NewInformer(lw,&v1.Pod{},time.Minute,&ActionHandler{})
	stopInformer  := make(chan struct{} )
	go informer.Run(stopInformer)


	fmt.Printf("informer begined")

	stopMain := make(chan os.Signal,1)

	signal.Notify(stopMain,syscall.SIGHUP,syscall.SIGINT,syscall.SIGQUIT)

	<- stopMain

}
