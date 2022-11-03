# Qiniu CSI Plugin

CSI Plugin for Kubernetes, Support Qiniu Cloud Storage.

## Introduction

Qiniu CSI Plugin implement an interface between CSI enabled Container Orchestrator and Qiniu Cloud Storage. You can create a PV with CSI configuration, and the PVC, Deployment defines as usual.

## Configuration Requirements

* Service Accounts with required RBAC permissions

## Compiling and Package

plugin.storage.qiniu.com can be compiled in a form of a container.

To build a container and push to Docker Hub:

```
$ make
```

## Usage

### Use Kodo CSI Plugin

#### Step 1: Create CSI Plugin

```sh
$ kubectl create -f ./k8s/kodo/
```

> Note: The plugin log style can be configured by environment variable: LOG_TYPE.

> "host": logs will be printed into files which save to host(/var/log/qiniu/storage/csi-plugin/kodoplugin.log);

> "stdout": logs will be printed to stdout, can be printed by docker logs or kubectl logs.

> "both": default option, log will be printed both to stdout and host file.

#### Step 2: Create PVC / Deploy with CSI Plugin

##### Static Provisioning

Fill out all CSI secret fields in ./examples/kodo/static-provisioning/secret.yaml

```sh
$ kubectl create -f ./examples/kodo/static-provisioning
$ kubectl create -f ./examples/kodo/deploy.yaml
```

##### Dynamic Provisioning（Enable IAM For your Kodo Account First）

Fill out all CSI secret fields in ./examples/kodo/dynamic-provisioning/secret.yaml

```sh
$ kubectl create -f ./examples/kodo/dynamic-provisioning
$ kubectl create -f ./examples/kodo/deploy.yaml
```

#### Step 3: Check status of PV / PVC

```sh
$ kubectl get pv | grep kodo
kodo-csi-pv   5Gi        RWX            Retain           Bound    default/kodo-pvc                           10m
$ kubectl get pvc | grep kodo
kodo-pvc   Bound    kodo-csi-pv   5Gi        RWX                           10m
```

#### Step 4: Try to read / write in the Volume of Kodo

```sh
$ kubectl get pods
NAME                                        READY   STATUS    RESTARTS   AGE
deployment-kodo-88bc7b647-7fx4x           1/1     Running   0          14m
$ kubectl exec -it deployment-kodo-88bc7b647-7fx4x -- sh -c 'echo "Hello Kodo" > /data/hello.txt'
$ kubectl exec -it deployment-kodo-88bc7b647-7fx4x -- sh -c 'cat /data/hello.txt'
Hello Kodo
$ kubectl exec -it deployment-kodo-88bc7b647-7fx4x -- sh -c 'time dd if=/dev/urandom of=/dev/stdout bs=1048576 count=1024 | tee >(md5sum) >/data/1g'
1024+0 records in
1024+0 records out
real    11m 30.29s
user    0m 0.00s
sys     0m 9.55s
0d4d49febda814cd92a362a1bba18091  -
$ kubectl exec -it deployment-kodofs-88bc7b647-7fx4x -- sh -c 'time md5sum /data/1g'
0d4d49febda814cd92a362a1bba18091  /data/1g
real    20m 18.28s
user    0m 3.03s
sys     0m 0.70s
```

#### Cache Mode

##### off

By default cache mode (off), the cache will read directly from the remote and write directly to the remote without caching anything on disk.

This will mean some operations are not possible

* Files can't be opened for both read AND write
* Files opened for write can't be seeked
* Existing files opened for write must have O_TRUNC set
* Files open for read with O_TRUNC will be opened write only
* Files open for write only will behave as if O_TRUNC was supplied
* Open modes O_APPEND, O_TRUNC are ignored
* If an upload fails it can't be retried

##### minimal

This is very similar to "off" except that files opened for read AND write will be buffered to disk. This means that files opened for write will be a lot more compatible, but uses the minimal disk space.

These operations are not possible

* Files opened for write only can't be seeked
* Existing files opened for write must have O_TRUNC set
* Files opened for write only will ignore O_APPEND, O_TRUNC
* If an upload fails it can't be retried

##### writes

In this mode files opened for read only are still read directly from the remote, write only and read/write files are buffered to disk first.

This mode should support all normal file system operations.

If an upload fails it will be retried at exponentially increasing intervals up to 1 minute.

##### full

In this mode all reads and writes are buffered to and from disk. When data is read from the remote this is buffered to disk as well.

In this mode the files in the cache will be sparse files and rclone will keep track of which bits of the files it has downloaded.

So if an application only reads the starts of each file, then rclone will only buffer the start of the file. These files will appear to be their full size in the cache, but they will be sparse files with only the data that has been downloaded present in them.

This mode should support all normal file system operations.

### Use KodoFS CSI Plugin

#### Step 1: Create CSI Plugin

```sh
$ kubectl create -f ./k8s/kodofs/
```

> Note: The plugin log style can be configured by environment variable: LOG_TYPE.

> "host": logs will be printed into files which save to host(/var/log/qiniu/storage/csi-plugin/kodofsplugin.log);

> "stdout": logs will be printed to stdout, can be printed by docker logs or kubectl logs.

> "both": default option, log will be printed both to stdout and host file.

#### Step 2: Create PVC / Deploy with CSI Plugin

##### Static Provisioning

Fill out all CSI secret fields in ./examples/kodofs/static-provisioning/secret.yaml

```sh
$ kubectl create -f ./examples/kodofs/static-provisioning
$ kubectl create -f ./examples/kodofs/deploy.yaml
```

##### Dynamic Provisioning

Fill out all CSI secret fields in ./examples/kodofs/dynamic-provisioning/secret.yaml

```sh
$ kubectl create -f ./examples/kodofs/dynamic-provisioning
$ kubectl create -f ./examples/kodofs/deploy.yaml
```

#### Step 3: Check status of PV / PVC

```sh
$ kubectl get pv | grep kodofs
kodofs-csi-pv   5Gi        RWX            Retain           Bound    default/kodofs-pvc                           10m
$ kubectl get pvc | grep kodofs
kodofs-pvc   Bound    kodofs-csi-pv   5Gi        RWX                           10m
```

#### Step 4: Try to read / write in the Volume of KodoFS

```sh
$ kubectl get pods
NAME                                        READY   STATUS    RESTARTS   AGE
deployment-kodofs-88bc7b647-7fx4x           1/1     Running   0          14m
$ kubectl exec -it deployment-kodofs-88bc7b647-7fx4x -- sh -c 'echo "Hello KodoFS" > /data/hello.txt'
$ kubectl exec -it deployment-kodofs-88bc7b647-7fx4x -- sh -c 'cat /data/hello.txt'
Hello KodoFS
$ kubectl exec -it deployment-kodofs-88bc7b647-7fx4x -- sh -c 'time dd if=/dev/urandom of=/dev/stdout bs=1048576 count=1024 | tee >(md5sum) >/data/1g'
1024+0 records in
1024+0 records out
real    1m 22.04s
user    0m 0.00s
sys     0m 24.42s
df8c1e334cd3fd85149f21dd69e32e1d  -
$ kubectl exec -it deployment-kodofs-88bc7b647-7fx4x -- sh -c 'time md5sum /data/1g'
df8c1e334cd3fd85149f21dd69e32e1d  /data/1g
real    0m 20.28s
user    0m 3.50s
sys     0m 0.76s
```
