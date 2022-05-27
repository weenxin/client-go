package main

import (
	"fmt"
	"io"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
)

// 博客地址：https://www.modb.pro/db/137716

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", "/Users/yulongxin/.kube/config")
	if err != nil {
		klog.Fatalf(fmt.Sprintf("create client failed : %s", err))
	}
	client := clientset.NewForConfigOrDie(config)
	req := client.CoreV1().RESTClient().Post().Namespace("default").
		Resource("pods").Name("nginx").SubResource("portforward")

	klog.Info(req.URL())

	signals := make(chan os.Signal, 1)
	StopChannel := make(chan struct{}, 1)
	ReadyChannel := make(chan struct{})

	defer signal.Stop(signals)

	signal.Notify(signals, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	go func() {
		<-signals
		if StopChannel != nil {
			close(StopChannel)
		}
	}()

	if err := ForwardPorts("POST", req.URL(), config, StopChannel, ReadyChannel); err != nil {
		klog.Fatalln(err)
	}

}

func ForwardPorts(method string, url *url.URL, config *rest.Config, StopChannel, ReadyChannel chan struct{}) error {
	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return err
	}
	address := []string{"0.0.0.0"}
	ports := []string{"8080:80"}

	IOStreams := struct {
		// In think, os.Stdin
		In io.Reader
		// Out think, os.Stdout
		Out io.Writer
		// ErrOut think, os.Stderr
		ErrOut io.Writer
	}{os.Stdin, os.Stdout, os.Stderr}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, method, url)
	fw, err := portforward.NewOnAddresses(dialer, address, ports, StopChannel, ReadyChannel, IOStreams.Out, IOStreams.ErrOut)
	if err != nil {
		return err
	}
	return fw.ForwardPorts()
}
