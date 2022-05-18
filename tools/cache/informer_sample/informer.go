package main

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
	accessor, err := meta.Accessor(obj)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("object added : %s, %s \n ", accessor.GetName() , informer.LastSyncResourceVersion())
}
func (handler *ActionHandler) OnUpdate(oldObj, newObj interface{}) {
	accessor, err := meta.Accessor(newObj)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("object updated : %s, %s \n ", accessor.GetName() , informer.LastSyncResourceVersion())
}

func (handler *ActionHandler) OnDelete(obj interface{}) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("object deleted : %s, %s \n ", accessor.GetName() , informer.LastSyncResourceVersion())
}


var informer cache.Controller


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

	_, informer = cache.NewInformer(lw,&v1.Pod{},time.Minute,&ActionHandler{})
	stopInformer  := make(chan struct{} )
	go informer.Run(stopInformer)


	fmt.Printf("informer begined")

	stopMain := make(chan os.Signal,1)

	signal.Notify(stopMain,syscall.SIGHUP,syscall.SIGINT,syscall.SIGQUIT)

	<- stopMain

}
