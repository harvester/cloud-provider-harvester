module github.com/harvester/harvester-cloud-provider

go 1.23.4

replace (
	github.com/google/cel-go => github.com/google/cel-go v0.12.7
	github.com/harvester/harvester => github.com/harvester/harvester v0.0.0-20240116061230-20e0fc24b786

	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20230124173128-2207cfed1803
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20230124173128-2207cfed1803

	k8s.io/api => k8s.io/api v0.26.10
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.26.10
	k8s.io/apimachinery => k8s.io/apimachinery v0.26.10
	k8s.io/apiserver => k8s.io/apiserver v0.26.10
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.26.10
	k8s.io/client-go => k8s.io/client-go v0.26.10
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.26.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.26.10
	k8s.io/code-generator => k8s.io/code-generator v0.26.10
	k8s.io/component-base => k8s.io/component-base v0.26.10
	k8s.io/component-helpers => k8s.io/component-helpers v0.26.10
	k8s.io/controller-manager => k8s.io/controller-manager v0.26.10
	k8s.io/cri-api => k8s.io/cri-api v0.26.10
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.26.10
	k8s.io/kms => k8s.io/kms v0.26.10
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.26.10
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.26.10
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.26.10
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.26.10
	k8s.io/kubectl => k8s.io/kubectl v0.26.10
	k8s.io/kubelet => k8s.io/kubelet v0.26.10
	k8s.io/kubernetes => k8s.io/kubernetes v1.25.9
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.26.10
	k8s.io/metrics => k8s.io/metrics v0.26.10
	k8s.io/mount-utils => k8s.io/mount-utils v0.26.10
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.26.10
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.26.10

	kubevirt.io/api => kubevirt.io/api v1.1.0
	kubevirt.io/client-go => kubevirt.io/client-go v1.1.0
	kubevirt.io/kubevirt => kubevirt.io/kubevirt v1.1.0

	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.4.9
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.14.7
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/harvester/harvester v1.2.1
	github.com/harvester/harvester-load-balancer v0.2.0-rc2
	github.com/rancher/wrangler v1.1.1
	github.com/sirupsen/logrus v1.9.2
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.29.0
	k8s.io/apimachinery v0.29.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/cloud-provider v0.26.10
	k8s.io/component-base v0.29.0
	k8s.io/klog/v2 v2.110.1
	kubevirt.io/api v1.1.0
	kubevirt.io/client-go v1.0.0
)

require (
	emperror.dev/errors v0.8.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2 // indirect
	github.com/adrg/xdg v0.3.1 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v1.4.10 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cisco-open/operator-tools v0.29.0 // indirect
	github.com/containernetworking/cni v1.1.2 // indirect
	github.com/containernetworking/plugins v1.1.1 // indirect
	github.com/coreos/go-iptables v0.6.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/coreos/prometheus-operator v0.38.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/deckarep/golang-set/v2 v2.1.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.8.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-kit/kit v0.10.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gobuffalo/flect v1.0.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.1.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/cel-go v0.17.7 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.0 // indirect
	github.com/harvester/harvester-network-controller v0.3.2 // indirect
	github.com/harvester/node-manager v0.1.5-0.20230614075852-de2da3ef3aca // indirect
	github.com/iancoleman/orderedmap v0.2.0 // indirect
	github.com/imdario/mergo v0.3.15 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jinzhu/copier v0.3.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k3s-io/helm-controller v0.11.7 // indirect
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.3.0 // indirect
	github.com/kube-logging/logging-operator/pkg/sdk v0.9.1 // indirect
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0 // indirect
	github.com/kubernetes/dashboard v1.10.1 // indirect
	github.com/longhorn/go-iscsi-helper v0.0.0-20231113050545-9df1e6b605c7 // indirect
	github.com/longhorn/longhorn-manager v1.5.3 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/moby/term v0.0.0-20220808134915-39b0c02b01ae // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/onsi/gomega v1.30.0 // indirect
	github.com/openshift/api v0.0.0 // indirect
	github.com/openshift/client-go v0.0.0 // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.63.0 // indirect
	github.com/prometheus/client_golang v1.18.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/rancher/aks-operator v1.0.7 // indirect
	github.com/rancher/apiserver v0.0.0-20230120214941-e88c32739dc7 // indirect
	github.com/rancher/dynamiclistener v0.3.5 // indirect
	github.com/rancher/eks-operator v1.1.5 // indirect
	github.com/rancher/fleet/pkg/apis v0.0.0-20230123175930-d296259590be // indirect
	github.com/rancher/gke-operator v1.1.4 // indirect
	github.com/rancher/kubernetes-provider-detector v0.1.5 // indirect
	github.com/rancher/lasso v0.0.0-20221227210133-6ea88ca2fbcc // indirect
	github.com/rancher/norman v0.0.0-20221205184727-32ef2e185b99 // indirect
	github.com/rancher/rancher v0.0.0-20230124173128-2207cfed1803 // indirect
	github.com/rancher/rancher/pkg/apis v0.0.0 // indirect
	github.com/rancher/remotedialer v0.2.6-0.20220624190122-ea57207bf2b8 // indirect
	github.com/rancher/rke v1.3.18 // indirect
	github.com/rancher/steve v0.0.0-20221209194631-acf9d31ce0dd // indirect
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20210727200656-10b094e30007 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/safchain/ethtool v0.0.0-20210803160452-9aa261dae9b1 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/tevino/tcp-shaker v0.0.0-20191112104505-00eab0aefc80 // indirect
	github.com/vishvananda/netlink v1.2.1-beta.2 // indirect
	github.com/vishvananda/netns v0.0.0-20211101163701-50045581ed74 // indirect
	go.etcd.io/etcd/api/v3 v3.5.9 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.9 // indirect
	go.etcd.io/etcd/client/v3 v3.5.9 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.35.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.44.0 // indirect
	go.opentelemetry.io/otel v1.19.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.19.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.19.0 // indirect
	go.opentelemetry.io/otel/metric v1.19.0 // indirect
	go.opentelemetry.io/otel/sdk v1.19.0 // indirect
	go.opentelemetry.io/otel/trace v1.19.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/crypto v0.16.0 // indirect
	golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/oauth2 v0.13.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/term v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.29.0 // indirect
	k8s.io/apiserver v0.29.0 // indirect
	k8s.io/component-helpers v0.27.1 // indirect
	k8s.io/controller-manager v0.27.1 // indirect
	k8s.io/gengo v0.0.0-20230829151522-9cce18d56c01 // indirect
	k8s.io/kms v0.28.3 // indirect
	k8s.io/kube-aggregator v0.26.4 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	k8s.io/kubectl v0.25.0 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	kubevirt.io/containerized-data-importer-api v1.57.0-alpha1 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	kubevirt.io/kubevirt v1.0.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.28.0 // indirect
	sigs.k8s.io/cli-utils v0.27.0 // indirect
	sigs.k8s.io/cluster-api v1.4.8 // indirect
	sigs.k8s.io/controller-runtime v0.14.7 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
