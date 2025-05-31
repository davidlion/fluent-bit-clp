# Setup local K8s cluster
## Install docker
Follow the guide here: https://docs.docker.com/engine/install/

## Install kubectl
`kubectl` is the command-line tool for interacting with Kubernetes clusters. You will use it to manage and inspect your k3d cluster.

Follow the guide here: https://kubernetes.io/docs/tasks/tools/#kubectl

## Install k3d
k3d is a lightweight wrapper to run k3s (Rancher Lab's minimal Kubernetes distribution) in docker.

Follow the guide here: https://k3d.io/stable/#installation

# Commands
## Create k8s cluster
```shell
# Start k8s with 1 server and 1 agent, and mount plugins local directory to the cluster
k3d cluster create yscope --servers 1 --agents 1 \
  -v <path to yscope fluent-bit-clp GitHub repo directory>/plugins:/plugins
```

## Deploy minio and log-viewer
```shell
kubectl apply -f minio.yaml
kubectl apply -f yscope-log-viewer-deployment.yaml -f aws-credentials.yaml
```

## Deploy fluent-bit dev container
```shell
# Fluent-bit configs are in the yaml file
kubectl apply -f fluent-bit-dev.yaml -f fluent-bit-dev-config.yaml -f aws-credentials.yaml

# To launch a shell into the fluent-bit container
kubectl exec -it fluent-bit-dev -c fluent-bit-dev -n default -- /bin/bash

# Test log collection
echo '{"message": "a log message"}' > /temp/test-0.log
# Afterwards, /tmp/path.ir.zstd file should be created containing compressed logs

# Inspect the logs for fluent-bit
kubectl logs fluent-bit-dev
# We should get the following
[2025/05/27 02:35:58] [ info] [input:tail:tail.0] inotify_fs_add(): inode=865010 watch_fd=1 name=/tmp/test.log
2025/05/27 02:36:03 [info] decoder.GetRecord error: EOF


# port forward
kubectl port-forward minio 9000:9000
```

## Delete cluster
```angular2html
k3d cluster delete yscope
```