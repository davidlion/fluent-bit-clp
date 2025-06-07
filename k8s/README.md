# k8s

## Setup local K8s cluster

### Install docker

Follow the guide here: [docker]

### Install kubectl

`kubectl` is the command-line tool for interacting with Kubernetes clusters. You will use it to
manage and inspect your k3d cluster.

Follow the guide here: [kubectl]

### Install k3d

k3d is a lightweight wrapper to run k3s (Rancher Lab's minimal Kubernetes distribution) in docker.

Follow the guide here: [k3d]

## Commands

### Create k8s cluster

```shell
# Start k8s with 1 server and 1 agent, and mount plugins local directory to the cluster
k3d cluster create yscope --servers 1 --agents 1 \
  -v <repo root directory>/prebuilt:/fluent-bit/plugins \
  -p 9000:30000@agent:0 \
  -p 9001:30001@agent:0
```

### Deploy minio and log-viewer

```shell
kubectl apply -f minio.yaml
kubectl apply -f yscope-log-viewer-deployment.yaml -f aws-credentials.yaml
kubectl apply -f logs-bucket-creation.yaml -f aws-credentials.yaml
```

### Deploy fluent-bit dev container

```shell
# Fluent-bit configs are in the yaml file
kubectl apply -f fluent-bit-dev.yaml -f fluent-bit-dev-config.yaml -f aws-credentials.yaml

# To launch a shell into the fluent-bit container
kubectl exec -it fluent-bit-dev -c fluent-bit-dev -n default -- /bin/bash

# Test log collection
echo '{"message": "a log message"}' > /tmp/test-0.log
# Afterwards, /tmp/compressed-logs.clp.zst file should be created containing compressed logs

# Inspect the logs for fluent-bit
kubectl logs fluent-bit-dev
# We should get the following
[2025/05/27 02:35:58] [info] [input:tail:tail.0] inotify_fs_add(): inode=865010 watch_fd=1 name=/tmp/test-0.log
2025/05/27 02:36:03 [info] decoder.GetRecord error: EOF

# port forward
kubectl port-forward minio 9000:9000
```

### Delete cluster

```angular2html
k3d cluster delete yscope
```

[docker]: https://docs.docker.com/engine/install
[k3d]: https://k3d.io/stable/#installation
[kubectl]: https://kubernetes.io/docs/tasks/tools/#kubectl
