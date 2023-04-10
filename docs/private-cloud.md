# 私有云使用 CSI 插件接入

对于私有云用户，可能需要进行一些额外的配置

## 配置域名解析

七牛 CSI 插件使用 S3 协议接入七牛对象存储，
S3 协议的 Virtual-Hosted Style Endpoint URL 必须需要通过规定格式的域名进行访问，
因此需要用户自行配置域名解析服务。

S3的域名规则如下：
    
```text
<bucket>.<region>.<domain>
```

其中`<bucket>`为存储空间名称，`<region>`为存储空间所在区域，`<domain>`为私有云对象存储的域名。

例如，存储空间名称为`test`，所在区域为`kodo1`，私有云对象存储的域名为`svc.cluster.local`，则 S3 的 Virtual-Hosted Style Endpoint URL 为`test.kodo1.svc.cluster.local`。对于静态存储卷，可直接添加一条普通域名解析记录即可。

对于动态存储卷，bucket名称为动态生成的，因此需要添加泛域名解析记录，例如`*.kodo1.svc.cluster.local`。


## 强制使用 Path Style URL

S3 协议的 Path Style Endpoint URL 规则为`<domain>/<bucket>`，其中`<domain>`为私有云对象存储的域名，`<bucket>`为存储空间名称。此处的domain也可以直接为私有云服务器的IP地址。

若用户的私有云环境未配置DNS服务器，可开启 `s3forcePathStyle` 开关，以 Path Style URL 的风格使用七牛 CSI 插件。

七牛 CSI 插件的 S3 Endpoint 的 URL Style 可通过如下几种方式任选一种进行配置：
1. 使用`static-provisioning`时，修改`pv.yaml`中的`volumeAttributes`配置参数选项`s3forcePathStyle`
2. 使用``dynamic-provisioning`时，修改`sc.yaml`中的`parameters`配置参数选项`s3forcePathStyle`
3. 修改`secret.yaml`中的`stringData`配置参数选项`s3forcePathStyle`
4. 修改`secret.yaml`中的`data`配置选项，注意`data`下的所有字符串数据必须是经过base64编码后的字符串数据
   
s3forcePathStyle选项可设置如下值：
1. 使用`s3forcePathStyle: 'false'` 来切换至 Virtual-Hosted Style URL
2. 使用`s3forcePathStyle: 'true'`来切换至 Path Style URL

默认在不配置该参数情况下`s3forcePathStyle`为`'true'`，私有云环境下使用 Path Style URL 更加友好。