# Bootstrapping the Kubernetes Worker Nodes

```bash
mkdir ../bootstrap-worker-node
cd ../bootstrap-worker-node
```

```bash
 for instance in worker0 worker1 ; do
    sshpass -p "$password" ssh root@${(P)instance} swapoff -a

    sshpass -p "$password" scp -o StrictHostKeyChecking=no ../binary/crictl-v1.21.0-linux-amd64.tar.gz root@${(P)instance}:~/
    sshpass -p "$password" scp -o StrictHostKeyChecking=no ../binary/runc.amd64 root@${(P)instance}:~/
    sshpass -p "$password" scp -o StrictHostKeyChecking=no ../binary/cni-plugins-linux-amd64-v0.9.1.tgz root@${(P)instance}:~/
    sshpass -p "$password" scp -o StrictHostKeyChecking=no ../binary/containerd-1.4.4-linux-amd64.tar.gz root@${(P)instance}:~/

    sshpass -p "$password" scp -o StrictHostKeyChecking=no ../binary/kubectl root@${(P)instance}:~/
    sshpass -p "$password" scp -o StrictHostKeyChecking=no ../binary/kube-proxy root@${(P)instance}:~/
    sshpass -p "$password" scp -o StrictHostKeyChecking=no ../binary/kubelet root@${(P)instance}:~/

 done
```

```bash
for instance in worker0 worker1 ; do
    sshpass -p "$password" ssh  root@${(P)instance}  mkdir -p \
      /etc/cni/net.d \
      /opt/cni/bin \
      /var/lib/kubelet \
      /var/lib/kube-proxy \
      /var/lib/kubernetes \
      /var/run/kubernetes


    sshpass -p "$password" ssh  root@${(P)instance} mkdir containerd
    sshpass -p "$password" ssh  root@${(P)instance} tar -xvf crictl-v1.21.0-linux-amd64.tar.gz
    sshpass -p "$password" ssh  root@${(P)instance} tar -xvf containerd-1.4.4-linux-amd64.tar.gz -C containerd
    sshpass -p "$password" ssh  root@${(P)instance} tar -xvf cni-plugins-linux-amd64-v0.9.1.tgz -C /opt/cni/bin/
    sshpass -p "$password" ssh  root@${(P)instance} mv runc.amd64 runc
    sshpass -p "$password" ssh  root@${(P)instance} chmod +x crictl kubectl kube-proxy kubelet runc
    sshpass -p "$password" ssh  root@${(P)instance} mv crictl kubectl kube-proxy kubelet runc /usr/local/bin/
    sshpass -p "$password" ssh  root@${(P)instance} "mv containerd/bin/* /bin/"

done
```

```bash
for instance in worker0 worker1 ; do

cidr=POD_CIDR_$instance


cat <<EOF |  tee 10-bridge.conf
{
    "cniVersion": "0.4.0",
    "name": "bridge",
    "type": "bridge",
    "bridge": "cnio0",
    "isGateway": true,
    "ipMasq": true,
    "ipam": {
        "type": "host-local",
        "ranges": [
          [{"subnet": "${(P)cidr}"}]
        ],
        "routes": [{"dst": "0.0.0.0/0"}]
    }
}
EOF

cat <<EOF |  tee 99-loopback.conf
{
    "cniVersion": "0.4.0",
    "name": "lo",
    "type": "loopback"
}
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no 10-bridge.conf root@${(P)instance}:/etc/cni/net.d/
sshpass -p "$password" scp -o StrictHostKeyChecking=no 99-loopback.conf root@${(P)instance}:/etc/cni/net.d/
done
```


```bash

for instance in worker0 worker1 ; do
sshpass -p "$password" ssh  root@${(P)instance}  sudo mkdir -p /etc/containerd/

cat << EOF | tee config.toml
[plugins]
  [plugins.cri]
    sandbox_image = "registry.aliyuncs.com/k8sxio/pause:3.2"
  [plugins.cri.containerd]
    snapshotter = "overlayfs"
    [plugins.cri.containerd.default_runtime]
      runtime_type = "io.containerd.runtime.v1.linux"
      runtime_engine = "/usr/local/bin/runc"
      runtime_root = ""
  [plugins."io.containerd.grpc.v1.cri"]
    sandbox_image = "registry.aliyuncs.com/k8sxio/pause:3.2"
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no config.toml root@${(P)instance}:/etc/containerd/


cat <<EOF | tee containerd.service
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target

[Service]
ExecStartPre=/sbin/modprobe overlay
ExecStart=/bin/containerd
Restart=always
RestartSec=5
Delegate=yes
KillMode=process
OOMScoreAdjust=-999
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity

[Install]
WantedBy=multi-user.target
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no containerd.service root@${(P)instance}:/etc/systemd/system/

sshpass -p "$password" ssh root@${(P)instance} systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance} systemctl enable containerd
sshpass -p "$password" ssh root@${(P)instance} systemctl restart containerd

done

```

### Configure the Kubelet

```bash
sshpass -p "$password" ssh root@${master0}  'cat <<EOF | kubectl apply --kubeconfig admin.kubeconfig -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations:
    rbac.authorization.kubernetes.io/autoupdate: "true"
  labels:
    kubernetes.io/bootstrapping: rbac-defaults
  name: system:kube-apiserver-to-kubelet
rules:
  - apiGroups:
      - ""
    resources:
      - nodes/proxy
      - nodes/stats
      - nodes/log
      - nodes/spec
      - nodes/metrics
    verbs:
      - "*"
EOF
'

sshpass -p "$password" ssh root@${master0}  'cat <<EOF | kubectl apply --kubeconfig admin.kubeconfig -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system:kube-apiserver
  namespace: ""
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:kube-apiserver-to-kubelet
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: kubernetes
EOF
'

```

```bash
for instance in worker0 worker1 ; do

cidr=POD_CIDR_$instance

    sshpass -p "$password" ssh root@${(P)instance}  sudo mv ${instance}-key.pem ${instance}.pem /var/lib/kubelet/
    sshpass -p "$password" ssh root@${(P)instance}  sudo mv ${instance}.kubeconfig /var/lib/kubelet/kubeconfig
    sshpass -p "$password" ssh root@${(P)instance}  sudo mv ca.pem /var/lib/kubernetes/

cat <<EOF | tee kubelet-config.yaml
kind: KubeletConfiguration
apiVersion: kubelet.config.k8s.io/v1beta1
authentication:
  anonymous:
    enabled: false
  webhook:
    enabled: true
  x509:
    clientCAFile: "/var/lib/kubernetes/ca.pem"
authorization:
  mode: Webhook
clusterDomain: "cluster.local"
clusterDNS:
  - "10.32.0.10"
podCIDR: "${(P)cidr}"
resolvConf: "/run/systemd/resolve/resolv.conf"
runtimeRequestTimeout: "15m"
tlsCertFile: "/var/lib/kubelet/${instance}.pem"
tlsPrivateKeyFile: "/var/lib/kubelet/${instance}-key.pem"
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no kubelet-config.yaml root@${(P)instance}:/var/lib/kubelet/


cat <<EOF |  tee kubelet.service
[Unit]
Description=Kubernetes Kubelet
Documentation=https://github.com/kubernetes/kubernetes
After=containerd.service
Requires=containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet \\
  --config=/var/lib/kubelet/kubelet-config.yaml \\
  --container-runtime=remote \\
  --container-runtime-endpoint=unix:///var/run/containerd/containerd.sock \\
  --image-pull-progress-deadline=2m \\
  --kubeconfig=/var/lib/kubelet/kubeconfig \\
  --network-plugin=cni \\
  --register-node=true \\
  --pod-infra-container-image=registry.aliyuncs.com/k8sxio/pause:3.2 \\
  --v=2
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no kubelet.service root@${(P)instance}:/etc/systemd/system/

sshpass -p "$password" ssh root@${(P)instance} systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance} systemctl enable kubelet
sshpass -p "$password" ssh root@${(P)instance} systemctl restart kubelet

done
```


### Configure the Kubernetes Proxy

```bash
for instance in worker0 worker1 ; do

sshpass -p "$password" ssh root@${(P)instance}  sudo mv kube-proxy.kubeconfig /var/lib/kube-proxy/kubeconfig

cat <<EOF | tee kube-proxy-config.yaml
kind: KubeProxyConfiguration
apiVersion: kubeproxy.config.k8s.io/v1alpha1
clientConnection:
  kubeconfig: "/var/lib/kube-proxy/kubeconfig"
mode: "iptables"
clusterCIDR: "10.200.0.0/16"
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no kube-proxy-config.yaml root@${(P)instance}:/var/lib/kube-proxy/


cat <<EOF | tee kube-proxy.service
[Unit]
Description=Kubernetes Kube Proxy
Documentation=https://github.com/kubernetes/kubernetes

[Service]
ExecStart=/usr/local/bin/kube-proxy \\
  --config=/var/lib/kube-proxy/kube-proxy-config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no kube-proxy.service root@${(P)instance}:/etc/systemd/system/


sshpass -p "$password" ssh root@${(P)instance}  systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance}  systemctl enable containerd kubelet kube-proxy
sshpass -p "$password" ssh root@${(P)instance}  systemctl start containerd kubelet kube-proxy
done

```


```bash
for instance in worker0 worker1 ; do
sshpass -p "$password" ssh root@${(P)instance}  systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance}  systemctl enable containerd kubelet kube-proxy
sshpass -p "$password" ssh root@${(P)instance}  systemctl restart containerd kubelet kube-proxy
done

```

验证
```
sshpass -p "$password" ssh root@${master0}  kubectl get nodes --kubeconfig admin.kubeconfig
```

## Configuring kubectl for Remote Access

```bash
KUBERNETES_PUBLIC_ADDRESS=$proxy

kubectl config set-cluster kubernetes-the-hard-way \
    --certificate-authority=../certs/ca.pem \
    --embed-certs=true \
    --server=https://${KUBERNETES_PUBLIC_ADDRESS}:6443

kubectl config set-credentials admin \
    --client-certificate=../certs/admin.pem \
    --client-key=../certs/admin-key.pem

kubectl config set-context kubernetes-the-hard-way \
    --cluster=kubernetes-the-hard-way \
    --user=admin

kubectl config use-context kubernetes-the-hard-way
```


```bash
kubectl version
```
