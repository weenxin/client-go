# Generating Kubernetes Configuration Files for Authentication

## The kubelet Kubernetes Configuration File

```bash
 for instance in worker0 worker1; do
   kubectl config set-cluster kubernetes-the-hard-way \
     --certificate-authority=ca.pem \
     --embed-certs=true \
     --server=https://${proxy}:6443 \
     --kubeconfig=${instance}.kubeconfig

   kubectl config set-credentials system:node:${instance} \
     --client-certificate=${instance}.pem \
     --client-key=${instance}-key.pem \
     --embed-certs=true \
     --kubeconfig=${instance}.kubeconfig

   kubectl config set-context default \
     --cluster=kubernetes-the-hard-way \
     --user=system:node:${instance} \
     --kubeconfig=${instance}.kubeconfig

   kubectl config use-context default --kubeconfig=${instance}.kubeconfig
 done
```

## The kube-proxy Kubernetes Configuration File

```bash
kubectl config set-cluster kubernetes-the-hard-way \
    --certificate-authority=ca.pem \
    --embed-certs=true \
    --server=https://${proxy}:6443 \
    --kubeconfig=kube-proxy.kubeconfig

  kubectl config set-credentials system:kube-proxy \
    --client-certificate=kube-proxy.pem \
    --client-key=kube-proxy-key.pem \
    --embed-certs=true \
    --kubeconfig=kube-proxy.kubeconfig

  kubectl config set-context default \
    --cluster=kubernetes-the-hard-way \
    --user=system:kube-proxy \
    --kubeconfig=kube-proxy.kubeconfig

  kubectl config use-context default --kubeconfig=kube-proxy.kubeconfig

```

## The kube-controller-manager Kubernetes Configuration File

```bash
kubectl config set-cluster kubernetes-the-hard-way \
    --certificate-authority=ca.pem \
    --embed-certs=true \
    --server=https://127.0.0.1:6443 \
    --kubeconfig=kube-controller-manager.kubeconfig

  kubectl config set-credentials system:kube-controller-manager \
    --client-certificate=kube-controller-manager.pem \
    --client-key=kube-controller-manager-key.pem \
    --embed-certs=true \
    --kubeconfig=kube-controller-manager.kubeconfig

  kubectl config set-context default \
    --cluster=kubernetes-the-hard-way \
    --user=system:kube-controller-manager \
    --kubeconfig=kube-controller-manager.kubeconfig

  kubectl config use-context default --kubeconfig=kube-controller-manager.kubeconfig
```

## The kube-scheduler Kubernetes Configuration File

```bash
kubectl config set-cluster kubernetes-the-hard-way \
    --certificate-authority=ca.pem \
    --embed-certs=true \
    --server=https://127.0.0.1:6443 \
    --kubeconfig=kube-scheduler.kubeconfig

  kubectl config set-credentials system:kube-scheduler \
    --client-certificate=kube-scheduler.pem \
    --client-key=kube-scheduler-key.pem \
    --embed-certs=true \
    --kubeconfig=kube-scheduler.kubeconfig

  kubectl config set-context default \
    --cluster=kubernetes-the-hard-way \
    --user=system:kube-scheduler \
    --kubeconfig=kube-scheduler.kubeconfig

kubectl config use-context default --kubeconfig=kube-scheduler.kubeconfig

```

## The admin Kubernetes Configuration File

```bash
kubectl config set-cluster kubernetes-the-hard-way \
    --certificate-authority=ca.pem \
    --embed-certs=true \
    --server=https://127.0.0.1:6443 \
    --kubeconfig=admin.kubeconfig

  kubectl config set-credentials admin \
    --client-certificate=admin.pem \
    --client-key=admin-key.pem \
    --embed-certs=true \
    --kubeconfig=admin.kubeconfig

  kubectl config set-context default \
    --cluster=kubernetes-the-hard-way \
    --user=admin \
    --kubeconfig=admin.kubeconfig

  kubectl config use-context default --kubeconfig=admin.kubeconfig

```

### Distribute the Kubernetes Configuration Files

```bash
for instance in worker0 worker1; do
    echo "processing ${instance}"
  sshpass -p "$password" scp -o StrictHostKeyChecking=no ${instance}.kubeconfig kube-proxy.kubeconfig root@${(P)instance}:~/
done
```

```bash
for instance in master0 master1 master2; do
  echo "processing ${instance}"
  sshpass -p "$password" scp -o StrictHostKeyChecking=no  admin.kubeconfig kube-controller-manager.kubeconfig kube-scheduler.kubeconfig root@${(P)instance}:~/
done
```

## Generating the Data Encryption Config and Key

```bash
ENCRYPTION_KEY=$(head -c 32 /dev/urandom | base64)
echo $ENCRYPTION_KEY
```


```bash
cat > encryption-config.yaml <<EOF
kind: EncryptionConfig
apiVersion: v1
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: ${ENCRYPTION_KEY}
      - identity: {}
EOF
```

```bash
for instance in master0 master1 master2; do
  echo "processing ${instance}"
  sshpass -p "$password" scp -o StrictHostKeyChecking=no  encryption-config.yaml root@${(P)instance}:~/
done
```