# Bootstrapping the Kubernetes Control Plane


```bash
mkdir ../bootstrap-control-plane
cd ../bootstrap-control-plane
```

```bash
for instance in master0 master1 master2; do
sshpass -p "$password" ssh root@${(P)instance}  mkdir -p /etc/kubernetes/config
done
```





```bash
for instance in master0 master1 master2; do
echo "transfer file to $instance"
sshpass -p "$password" scp -o StrictHostKeyChecking=no  ../binary/kube-apiserver ../binary/kube-controller-manager  ../binary/kube-scheduler ../binary/kubectl root@${(P)instance}:/usr/local/bin/
done

```

```bash
for instance in master0 master1 master2; do
sshpass -p "$password" ssh root@${(P)instance} mkdir -p /var/lib/kubernetes/
sshpass -p "$password" ssh root@${(P)instance} mv ca.pem ca-key.pem kubernetes-key.pem kubernetes.pem \
                                                   service-account-key.pem service-account.pem \
                                                   encryption-config.yaml /var/lib/kubernetes/

INTERNAL_IP=${(P)instance}
KUBERNETES_PUBLIC_ADDRESS=$proxy
cat <<EOF | tee kube-apiserver.service
[Unit]
Description=Kubernetes API Server
Documentation=https://github.com/kubernetes/kubernetes

[Service]
ExecStart=/usr/local/bin/kube-apiserver \\
  --advertise-address=$INTERNAL_IP \\
  --allow-privileged=true \\
  --apiserver-count=3 \\
  --audit-log-maxage=30 \\
  --audit-log-maxbackup=3 \\
  --audit-log-maxsize=100 \\
  --audit-log-path=/var/log/audit.log \\
  --authorization-mode=Node,RBAC \\
  --bind-address=0.0.0.0 \\
  --client-ca-file=/var/lib/kubernetes/ca.pem \\
  --enable-admission-plugins=NamespaceLifecycle,NodeRestriction,LimitRanger,ServiceAccount,DefaultStorageClass,ResourceQuota \\
  --etcd-cafile=/var/lib/kubernetes/ca.pem \\
  --etcd-certfile=/var/lib/kubernetes/kubernetes.pem \\
  --etcd-keyfile=/var/lib/kubernetes/kubernetes-key.pem \\
  --etcd-servers=https://$master0:2379,https://$master1:2379,https://$master2:2379 \\
  --event-ttl=1h \\
  --encryption-provider-config=/var/lib/kubernetes/encryption-config.yaml \\
  --kubelet-certificate-authority=/var/lib/kubernetes/ca.pem \\
  --kubelet-client-certificate=/var/lib/kubernetes/kubernetes.pem \\
  --kubelet-client-key=/var/lib/kubernetes/kubernetes-key.pem \\
  --runtime-config=api/all=true \\
  --service-account-key-file=/var/lib/kubernetes/service-account.pem \\
  --service-account-signing-key-file=/var/lib/kubernetes/service-account-key.pem \\
  --service-account-issuer=https://$KUBERNETES_PUBLIC_ADDRESS:6443 \\
  --service-cluster-ip-range=10.32.0.0/24 \\
  --service-node-port-range=30000-32767 \\
  --tls-cert-file=/var/lib/kubernetes/kubernetes.pem \\
  --tls-private-key-file=/var/lib/kubernetes/kubernetes-key.pem \\
  --v=2
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no kube-apiserver.service  root@${(P)instance}:/etc/systemd/system/

sshpass -p "$password" ssh root@${(P)instance} systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance} systemctl enable kube-apiserver
sshpass -p "$password" ssh root@${(P)instance} systemctl restart kube-apiserver
done

```


### Configure the Kubernetes Controller Manager

```bash
for instance in master0 master1 master2; do
sshpass -p "$password" ssh root@${(P)instance}  mv kube-controller-manager.kubeconfig /var/lib/kubernetes/

cat <<EOF | tee kube-controller-manager.service
[Unit]
Description=Kubernetes Controller Manager
Documentation=https://github.com/kubernetes/kubernetes

[Service]
ExecStart=/usr/local/bin/kube-controller-manager \\
  --bind-address=0.0.0.0 \\
  --cluster-cidr=10.200.0.0/16 \\
  --cluster-name=kubernetes \\
  --cluster-signing-cert-file=/var/lib/kubernetes/ca.pem \\
  --cluster-signing-key-file=/var/lib/kubernetes/ca-key.pem \\
  --kubeconfig=/var/lib/kubernetes/kube-controller-manager.kubeconfig \\
  --leader-elect=true \\
  --root-ca-file=/var/lib/kubernetes/ca.pem \\
  --service-account-private-key-file=/var/lib/kubernetes/service-account-key.pem \\
  --service-cluster-ip-range=10.32.0.0/24 \\
  --use-service-account-credentials=true \\
  --v=2
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sshpass -p "$password" scp -o StrictHostKeyChecking=no kube-controller-manager.service  root@${(P)instance}:/etc/systemd/system/

sshpass -p "$password" ssh root@${(P)instance} systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance} systemctl enable kube-controller-manager.service
sshpass -p "$password" ssh root@${(P)instance} systemctl restart kube-controller-manager.service

done
```

### Configure the Kubernetes Scheduler

```bash

cat <<EOF | tee kube-scheduler.yaml
apiVersion: kubescheduler.config.k8s.io/v1beta1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "/var/lib/kubernetes/kube-scheduler.kubeconfig"
leaderElection:
  leaderElect: true
EOF

cat <<EOF | tee kube-scheduler.service
[Unit]
Description=Kubernetes Scheduler
Documentation=https://github.com/kubernetes/kubernetes

[Service]
ExecStart=/usr/local/bin/kube-scheduler \\
  --config=/etc/kubernetes/config/kube-scheduler.yaml \\
  --v=2
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

for instance in master0 master1 master2; do
sshpass -p "$password" scp -o StrictHostKeyChecking=no kube-scheduler.yaml  root@${(P)instance}:/etc/kubernetes/config/

sshpass -p "$password" ssh root@${(P)instance}  mv kube-scheduler.kubeconfig /var/lib/kubernetes/

sshpass -p "$password" scp -o StrictHostKeyChecking=no kube-scheduler.service  root@${(P)instance}:/etc/systemd/system/

sshpass -p "$password" ssh root@${(P)instance} systemctl daemon-reload
sshpass -p "$password" ssh root@${(P)instance} systemctl enable kube-scheduler
sshpass -p "$password" ssh root@${(P)instance} systemctl restart kube-scheduler

done

```


检查组件

```bash

for instance in master0 master1 master2; do
sshpass -p "$password" ssh root@${(P)instance} chmod +x /usr/local/bin/kubectl
sshpass -p "$password" ssh root@${(P)instance} kubectl get componentstatuses --kubeconfig ./admin.kubeconfig
done

```