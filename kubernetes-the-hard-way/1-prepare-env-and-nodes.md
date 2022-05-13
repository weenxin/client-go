# 准备工作

**Prerequisite**
- sshpass ,ssh
- cfssl , cfssljson


## 节点规划

```
worker0=10.192.219.82
worker1=10.192.219.83
master0=10.192.219.84
master1=10.192.219.85
master2=10.192.219.86
proxy=10.192.219.87
password=ASDqwe123
POD_CIDR_worker0=10.200.0.0/24
POD_CIDR_worker1=10.200.1.0/24
```


## 准备节点

```bash
for instance in worker0 worker1 master0 master1 master2 ; do
sshpass -p "$password" ssh -o StrictHostKeyChecking=no  root@${(P)instance}  echo "hello"
done
```


```bash
for instance in worker0 worker1 ; do
sshpass -p "$password" ssh -o StrictHostKeyChecking=no  root@${(P)instance}  mkdir -p /run/systemd/resolve
sshpass -p "$password" ssh -o StrictHostKeyChecking=no  root@${(P)instance}  ln -s /etc/resolv.conf /run/systemd/resolve/resolv.conf
done
```

使用`kubelog`时需要master需要从worker获取，增加host信息。

```bash
for master in master0 master1 master2; do
    for instance in worker0 worker1; do
        sshpass -p "$password" ssh  -o StrictHostKeyChecking=no   root@${(P)master} "echo \"${(P)instance}  $instance \" >> /etc/hosts"
    done
done
```


```bash
for instance in worker0 worker1; do
  sshpass -p "$password" ssh root@${(P)instance}  hostnamectl set-hostname ${instance}
done
```

```bash
for instance in worker0 worker1 master0 master1 master2 proxy; do
sshpass -p "$password" ssh  -o StrictHostKeyChecking=no   root@${(P)instance} "yum update -y"
done
```

```bash
for instance in worker0 worker1 ; do
    sshpass -p "$password" ssh root@${(P)instance} yum install -y install socat conntrack ipset
done
```

```bash
mkdir certs binary network etcd
```

```bash
cd binary

wget -q --show-progress --https-only --timestamping \
  "https://github.com/etcd-io/etcd/releases/download/v3.4.15/etcd-v3.4.15-linux-amd64.tar.gz"

wget -q --show-progress --https-only --timestamping \
  "https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kube-apiserver" \
  "https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kube-controller-manager" \
  "https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kube-scheduler" \
  "https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl"




wget -q --show-progress --https-only --timestamping \
  https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.21.0/crictl-v1.21.0-linux-amd64.tar.gz \
  https://github.com/opencontainers/runc/releases/download/v1.0.0-rc93/runc.amd64 \
  https://github.com/containernetworking/plugins/releases/download/v0.9.1/cni-plugins-linux-amd64-v0.9.1.tgz \
  https://github.com/containerd/containerd/releases/download/v1.4.4/containerd-1.4.4-linux-amd64.tar.gz \
  https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl \
  https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kube-proxy \
  https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubelet


chmod +x kube-apiserver kube-controller-manager kube-scheduler kubectl kube-proxy kubelet


```


