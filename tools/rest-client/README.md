# Rest-client

## Kube-Config

在工作中，我们可能需要同时与多个Kubernetes集群交流，大部分情况下，我会通过一个Kubernetes config 管理集群信息的。那么Kubernetes Config中保存了那些内容呢 ？配置文件在程序中的结构是如何的呢 ？

```go
type Config struct {
	// Legacy field from pkg/api/types.go TypeMeta.
	// TODO(jlowdermilk): remove this after eliminating downstream dependencies.
	// +k8s:conversion-gen=false
	// +optional
	Kind string `json:"kind,omitempty"`
	// Legacy field from pkg/api/types.go TypeMeta.
	// TODO(jlowdermilk): remove this after eliminating downstream dependencies.
	// +k8s:conversion-gen=false
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Preferences holds general information to be use for cli interactions
	Preferences Preferences `json:"preferences"`
	// Clusters is a map of referencable names to cluster configs
	// 集群信息，集群地址等信息
	Clusters map[string]*Cluster `json:"clusters"`
	// AuthInfos is a map of referencable names to user configs
	// 用户信息， 认证信息
	AuthInfos map[string]*AuthInfo `json:"users"`
	// Contexts is a map of referencable names to context configs
	// 将集群信息和认证信息组合在一起形成了一个context
	Contexts map[string]*Context `json:"contexts"`
	// CurrentContext is the name of the context that you would like to use by default
	//当前使用的context
	CurrentContext string `json:"current-context"`
	// Extensions holds additional information. This is useful for extenders so that reads and writes don't clobber unknown fields
	// +optional
	Extensions map[string]runtime.Object `json:"extensions,omitempty"`
}

```

- 一个`config`由多个context组成
- 一个`context` 由一个集群信息和用户认证信息组成
- 集群信息和用户认证信息独立维护；


## Clientcmd

在`clientcmd`包中有关于kubeconfig相关的内容。包的主要用途是为了生成`rest.Config`.

```go
func BuildConfigFromFlags(masterUrl, kubeconfigPath string) (*restclient.Config, error) {
	if kubeconfigPath == "" && masterUrl == "" {
		klog.Warning("Neither --kubeconfig nor --master was specified.  Using the inClusterConfig.  This might not work.")
		kubeconfig, err := restclient.InClusterConfig()
		if err == nil {
			return kubeconfig, nil
		}
		klog.Warning("error creating inClusterConfig, falling back to default config: ", err)
	}
	return NewNonInteractiveDeferredLoadingClientConfig(
		&ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterUrl}}).ClientConfig()
}
```

## rest

rest包可以建立与Kubernetes连接的client。比如给予`clientcmd`建立的config，可以初始化一个全局的client。

```go
client, err := kubernetes.NewForConfig(config)
```

基于创建的client可以对kubernete做各种各样的操作比如建立一个pod。


### Create

```go
//create a pod
	_, err = client.CoreV1().Pods(DefaultNamespace).Create(context.Background(), &v1.Pod{
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
				},
			},
		},
	},
		meta_v1.CreateOptions{},
	)
```

### Get

```go
pod, err = client.CoreV1().Pods(DefaultNamespace).Get(context.Background(), PodName, meta_v1.GetOptions{})
	if err != nil {
		panic(fmt.Sprintf("can not get pod , err : %s", err))
	}
```

### Update


```go
pod.Spec.Containers[0].Image = "busybox:1.35.0"
	pod, err = client.CoreV1().Pods(DefaultNamespace).Update(context.Background(), pod, meta_v1.UpdateOptions{})
	if err != nil {
		panic(fmt.Sprintf("can not upate pod, err : %s", err))
	}
```


### List Pod

```go
//测试listPod
	pods, err := client.CoreV1().Pods(DefaultNamespace).List(context.Background(), meta_v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ServiceName: PodName}).String(),
	})
	fmt.Printf("pods count : %v \n", len(pods.Items))

	for index, pod := range pods.Items {
		fmt.Printf("pods[%d].name is %s \n", index, pod.Name)
	}
```

### Subsource/Log

```go

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
```


### Subresouce/Exec

```go
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
```





