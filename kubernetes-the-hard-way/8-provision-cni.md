# Provision A CNI


```bash
mkdir ../cni
cd ../cni
```


```bash
先更改下clusterCIDR，在provision
kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/master/Documentation/kube-flannel.yml
```

发现flannel的Pod一直在重启；

查看日志

```bash
kubectl logs {flannel-pod-name} -n kube-system
```

发现报错：

```
Error from server: Get "https://worker0:10250/containerLogs/kube-system/kube-flannel-ds-k7qk8/kube-flannel": dial tcp: lookup worker0 on 100.100.2.136:53: no such host
```

这是因为master尝试按照worker0这个域名查看的时候，由于master host文件中没有该域名，所以链接到了莫名地址。10250是kubelet的端口。 加入worker host到master节点中。


再次查看，继续查看日志

```bash
kubectl logs {flannel-pod-name} -n kube-system
```

```
I0512 10:09:45.305447       1 main.go:205] CLI flags config: {etcdEndpoints:http://127.0.0.1:4001,http://127.0.0.1:2379 etcdPrefix:/coreos.com/network etcdKeyfile: etcdCertfile: etcdCAFile: etcdUsername: etcdPassword: version:false kubeSubnetMgr:true kubeApiUrl: kubeAnnotationPrefix:flannel.alpha.coreos.com kubeConfigFile: iface:[] ifaceRegex:[] ipMasq:true subnetFile:/run/flannel/subnet.env publicIP: publicIPv6: subnetLeaseRenewMargin:60 healthzIP:0.0.0.0 healthzPort:0 iptablesResyncSeconds:5 iptablesForwardRules:true netConfPath:/etc/kube-flannel/net-conf.json setNodeNetworkUnavailable:true}
W0512 10:09:45.305546       1 client_config.go:614] Neither --kubeconfig nor --master was specified.  Using the inClusterConfig.  This might not work.
I0512 10:09:45.407273       1 kube.go:120] Waiting 10m0s for node controller to sync
I0512 10:09:45.407331       1 kube.go:378] Starting kube subnet manager
I0512 10:09:46.408044       1 kube.go:127] Node controller sync successful
I0512 10:09:46.408068       1 main.go:225] Created subnet manager: Kubernetes Subnet Manager - worker0
I0512 10:09:46.408074       1 main.go:228] Installing signal handlers
I0512 10:09:46.408249       1 main.go:454] Found network config - Backend type: vxlan
I0512 10:09:46.408305       1 match.go:189] Determining IP address of default interface
I0512 10:09:46.408558       1 match.go:242] Using interface with name eth0 and address 10.192.220.94
I0512 10:09:46.408575       1 match.go:264] Defaulting external address to interface address (10.192.220.94)
I0512 10:09:46.408909       1 vxlan.go:138] VXLAN config: VNI=1 Port=0 GBP=false Learning=false DirectRouting=false
E0512 10:09:46.409247       1 main.go:317] Error registering network: failed to acquire lease: node "worker0" pod cidr not assigned
W0512 10:09:46.409783       1 reflector.go:436] github.com/flannel-io/flannel/subnet/kube/kube.go:379: watch of *v1.Node ended with: an error on the server ("unable to decode an event from the watch stream: context canceled") has prevented the request from succeeding
I0512 10:09:46.409829       1 main.go:434] Stopping shutdownHandler...
```


发现：

```bash
E0512 10:09:46.409247       1 main.go:317] Error registering network: failed to acquire lease: node "worker0" pod cidr not assigned
```

在Kubernetes中有三种网络：

- ClusterCIDR : 所有Pod可以分配到的IP范围
- PodCIDR： 节点可以分配的IP范围
- ServiceCIDR： Kubernetes集群内部服务的IP范围

查看node的信息

```bash
kubectl get node -o yaml
```

```yaml
apiVersion: v1
kind: Node
metadata:
  annotations:
    flannel.alpha.coreos.com/backend-data: '{"VNI":1,"VtepMAC":"ee:43:f0:12:f0:3f"}'
    flannel.alpha.coreos.com/backend-type: vxlan
    flannel.alpha.coreos.com/kube-subnet-manager: "true"
    flannel.alpha.coreos.com/public-ip: 10.192.220.94
    node.alpha.kubernetes.io/ttl: "0"
    volumes.kubernetes.io/controller-managed-attach-detach: "true"
  creationTimestamp: "2022-05-12T08:56:49Z"
  labels:
    beta.kubernetes.io/arch: amd64
    beta.kubernetes.io/os: linux
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: worker0
    kubernetes.io/os: linux
  name: worker0
  resourceVersion: "15356"
  uid: ec7317d3-787c-41c0-aa7b-fcb9a927b449
spec: {}
```

发现确实，没有PodCIDR的信息，加入信息。

```yaml
apiVersion: v1
kind: Node
metadata:
  annotations:
    flannel.alpha.coreos.com/backend-data: '{"VNI":1,"VtepMAC":"ee:43:f0:12:f0:3f"}'
    flannel.alpha.coreos.com/backend-type: vxlan
    flannel.alpha.coreos.com/kube-subnet-manager: "true"
    flannel.alpha.coreos.com/public-ip: 10.192.220.94
    node.alpha.kubernetes.io/ttl: "0"
    volumes.kubernetes.io/controller-managed-attach-detach: "true"
  creationTimestamp: "2022-05-12T08:56:49Z"
  labels:
    beta.kubernetes.io/arch: amd64
    beta.kubernetes.io/os: linux
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: worker0
    kubernetes.io/os: linux
  name: worker0
  resourceVersion: "15356"
  uid: ec7317d3-787c-41c0-aa7b-fcb9a927b449
spec:
  podCIDR: 10.200.0.0/24
  podCIDRs:
  - 10.200.0.0/24
```

此时网络应该是可以了。
