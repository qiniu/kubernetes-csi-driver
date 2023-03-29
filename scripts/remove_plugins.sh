kubectl delete -n kube-system \
    csidrivers.storage.k8s.io kodoplugin.storage.qiniu.com

kubectl delete -n kube-system \
    daemonsets.apps kodo-csi-plugin

kubectl delete -n kube-system \
    deployments.apps kodo-provisioner 

kubectl delete -n kube-system \
    clusterrolebindings.rbac.authorization.k8s.io binding.kodoplugin.storage.qiniu.com

kubectl delete -n kube-system \
    clusterroles.rbac.authorization.k8s.io role.kodoplugin.storage.qiniu.com
    
kubectl delete -n kube-system \
    serviceaccounts sa.kodoplugin.storage.qiniu.com
