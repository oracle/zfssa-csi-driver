module github.com/oracle/zfssa-csi-driver

go 1.13

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/protobuf v1.4.0
	github.com/kubernetes-csi/csi-lib-iscsi v0.0.0-20190415173011-c545557492f4
	github.com/kubernetes-csi/csi-lib-utils v0.6.1
	github.com/onsi/gomega v1.9.0 // indirect
	github.com/prometheus/client_golang v1.2.1 // indirect
	golang.org/x/net v0.0.0-20191101175033-0deb6923b6d9
	golang.org/x/sys v0.0.0-20200223170610-d5e6a3e2c0ae // indirect
	google.golang.org/grpc v1.23.1
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/apimachinery v0.17.11
	k8s.io/client-go v0.18.2
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.17.5
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
)

replace (
	k8s.io/api => k8s.io/api v0.17.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.6-beta.0
	k8s.io/apiserver => k8s.io/apiserver v0.17.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.17.5
	k8s.io/client-go => k8s.io/client-go v0.17.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.17.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.17.5
	k8s.io/code-generator => k8s.io/code-generator v0.17.6-beta.0
	k8s.io/component-base => k8s.io/component-base v0.17.5
	k8s.io/cri-api => k8s.io/cri-api v0.17.13-rc.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.17.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.17.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.17.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.17.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.17.5
	k8s.io/kubelet => k8s.io/kubelet v0.17.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.17.5
	k8s.io/metrics => k8s.io/metrics v0.17.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.17.5
)

replace k8s.io/kubectl => k8s.io/kubectl v0.17.5

replace k8s.io/node-api => k8s.io/node-api v0.17.5

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.17.5

replace k8s.io/sample-controller => k8s.io/sample-controller v0.17.5
