# 私有云使用 CSI 插件接入

对于私有云用户，可能需要进行一些额外的配置

## 配置域名解析

七牛 CSI 插件使用 S3 协议接入七牛对象存储，
S3 协议的 Virtual-Hosted Style Endpoint URL 必须需要通过规定格式的域名进行访问，
因此需要用户自行配置域名解析服务。

k8s 提供了 coredns 插件，
可以通过配置 coredns 来为k8s集群添加全局自定义域名解析服务：

编辑 coredns 配置:
```sh
kubectl -n kube-system edit configmap coredns
```

寻找到 hosts 配置所在的段落，在其中加入自定义的 s3 域名解析规则:
```text
      hosts {
          ...
          10.10.10.10 *.<region>.aaaaaa.bbb
          fallthrough
      }
```

## 强制使用 Path Style URL

七牛 CSI 插件的 S3 Endpoint 的 URL Style 可通过调整pv.yaml中的csi的volumeAttributes配置参数选项`s3forcePathStyle`进行改变。
1. 使用`s3forcePathStyle: 'false'` 来切换至 Virtual-Hosted Style URL
2. 使用`s3forcePathStyle: 'true'`来切换至 Path Style URL

也可以在secret.yaml中的stringData或data中进行配置该参数具有相同的效果，
默认在不配置该参数情况下s3forcePathStyle为启用状态，私有云环境下使用 Path Style URL 更加友好。