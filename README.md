# TCP retransmission timeout issue

Github issue https://github.com/projectcalico/calico/issues/7983

This repository contains a simple client/server application that can be used to reproduce the following issue:

If a server pod is abruptly deleted it should cause the client to receive a `TCP RST` message, when the client attempts to send data to the server that was previously terminated.
In certain conditions, demonstrated by the example in this repository, this does not happen.
Instead, a long sequence of TCP retransmits will begin and the client will block for 15 minutes until TCP retransmission timeout is reached.

NOTE: This issue seems to be specific to certain deployments of Kubernetes, possibly related to Calico implementation.
Clusters with Kindnet or Flannel CNI plugins do not seem to exhibit the issue.


## Preconditions

The client and server applications [`docker/echo/main.go`](docker/echo/main.go) work as follows:

The client application establishes a TCP connection to the server.
The server application accepts the TCP connection.
The client application periodically (every 5 seconds) sends data to the server, and the server application echoes the data back.
This continues until an error happens on either side.

The following conditions need to be met to reproduce the issue:

- Client and server pods are running in different nodes of the cluster (e.g. with pod anti-affinity).
- The server pod is exposed to the client pod using a service of type ClusterIP, but NOT headless.
- The server pod is force-deleted.
- It seems to make the issue 100% reproducible if the server application has set a signal handler to ignore `SIGTERM` e.g. for graceful shutdown purposes.


## Reproduction steps

Optional:
If you want to reproduce the issue on Kind cluster, first run the following command to create a cluster with 2 worker nodes with Calico.

```
kind create cluster --config configs/kind-cluster-config.yaml --name echo
```

The configuration file [`configs/kind-cluster-config.yaml`](configs/kind-cluster-config.yaml) disables the default CNI plugin (kindnet).
To install Calico run the following command (see details [here](https://docs.tigera.io/calico/latest/getting-started/kubernetes/kind)).

```
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml
```

Next, deploy the client and server applications to the cluster.

```
kubectl apply -f manifests/echo.yaml
```

Observe that the client and server pods are running on different nodes of the cluster.

```
kubectl get pods -o wide
```

Use the test application logs to observe the following

- TCP connection is established between the client and server.
- The client is periodically sending data to the server.
- The server is echoing the data back to the client.

```
kubectl logs deployment/client
kubectl logs deployment/server
```

Next, force-delete the server pod.

```
kubectl delete pod -l app=server --force
```

Observe that the server pod is deleted immediately but the server process continues to run for a few seconds since it ignores the `SIGTERM` signal.
It may not be a requirement, but it seems to help to reproduce the issue reliably.
In that case, the TCP connection is still established and the client and server continue sending data after the pod got deleted.

```console
$ kubectl logs deployment/client -f
2023/09/01 17:54:21 INFO Client started
2023/09/01 17:54:26 INFO Connecting to server remote_addr=server:8000
2023/09/01 17:54:26 INFO Sending request remote_addr=10.96.224.117:8000 local_addr=192.168.75.2:42056
2023/09/01 17:54:26 INFO Received response remote_addr=10.96.224.117:8000 local_addr=192.168.75.2:42056
...
2023/09/01 17:55:16 INFO Sending request remote_addr=10.96.224.117:8000 local_addr=192.168.75.2:42056   # <-- Server pod is deleted here
```

At the last line `17:55:16 INFO Sending request` the client gets blocked while reading the response from the socket, which never arrives.
It can be observed with Wireshark that a sequence of TCP retransmissions begins.
It will continue until the TCP retransmit timeout is finally reached after 15 minutes (see discussion [here](https://pracucci.com/linux-tcp-rto-min-max-and-tcp-retries2.html)).
The client application receives an error.

```console
2023/09/01 18:10:55 ERROR Error reading data error="read tcp 192.168.75.2:42056->10.96.224.117:8000: read: connection timed out"
2023/09/01 18:10:55 INFO Connection closed address=10.96.224.117:8000
```

## Further observations

The following conntrack entry can be seen on the worker node where the client pod is running, even after the server process has been killed.

``` console
$ conntrack -L | grep 10.96.224.117
tcp      6 278 ESTABLISHED src=192.168.75.2 dst=10.96.224.117 sport=42056 dport=8000 src=192.168.136.2 dst=192.168.75.2 sport=8000 dport=42056 [ASSURED] mark=0 use=1
```

Waiting for 15 minutes resolves the issue, but the conntrack entry remains even after that.
Restarting the client pod removes the conntrack entry.

The issue does not happen if the client and server pods are running in the same node.

The issue might be less likely to happen if the server does not set a signal handler to ignore `SIGTERM` signal.
To test this, edit the [`manifests/echo.yaml`](manifests/echo.yaml) file and comment out the line with `--catch-sigterm` from the server command line.
Then redeploy the application.


## Build the test application locally

Pre-built image is available at [`quay.io/tsaarni/echo:latest`](https://quay.io/tsaarni/echo:latest).
To build the application locally, run the following command.

```
docker build -t localhost/echo:latest docker/echo
```
