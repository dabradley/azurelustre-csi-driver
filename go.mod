module sigs.k8s.io/azurelustre-csi-driver

go 1.23

require (
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.1.1
	github.com/Azure/go-autorest/autorest v0.11.28
	github.com/container-storage-interface/spec v1.8.0
	github.com/kubernetes-csi/csi-lib-utils v0.9.1
	github.com/kubernetes-csi/csi-test/v5 v5.1.0
	github.com/pborman/uuid v1.2.0
	github.com/pelletier/go-toml v1.9.4
	github.com/stretchr/testify v1.8.3
	golang.org/x/net v0.23.0
	google.golang.org/grpc v1.58.3
	google.golang.org/protobuf v1.33.0
	k8s.io/apimachinery v0.26.11
	k8s.io/klog/v2 v2.100.1
	k8s.io/kubernetes v1.26.11
	k8s.io/mount-utils v0.26.9
	k8s.io/utils v0.0.0-20230406110748-d93618cff8a2
	sigs.k8s.io/cloud-provider-azure v1.26.3
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/Azure/go-autorest/autorest/adal v0.9.21 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.0.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
)

require (
	github.com/Azure/azure-sdk-for-go v67.3.0+incompatible // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.8.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.4.0
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.3.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/mocks v0.4.2 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo/v2 v2.12.0 // indirect
	github.com/onsi/gomega v1.27.10 // indirect
	github.com/opencontainers/selinux v1.10.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.16.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.10.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/crypto v0.21.0 // indirect
	golang.org/x/oauth2 v0.10.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/term v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.12.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/api v0.26.11 // indirect
	k8s.io/apiserver v0.26.11 // indirect
	k8s.io/client-go v0.26.11 // indirect
	k8s.io/cloud-provider v0.26.0 // indirect
	k8s.io/component-base v0.26.11 // indirect
	k8s.io/component-helpers v0.26.11 // indirect
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace k8s.io/api => k8s.io/api v0.26.11

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.26.11

replace k8s.io/apimachinery => k8s.io/apimachinery v0.26.11

replace k8s.io/apiserver => k8s.io/apiserver v0.26.11

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.26.11

replace k8s.io/client-go => k8s.io/client-go v0.26.11

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.26.11

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.26.11

replace k8s.io/code-generator => k8s.io/code-generator v0.26.11

replace k8s.io/component-base => k8s.io/component-base v0.26.11

replace k8s.io/component-helpers => k8s.io/component-helpers v0.26.11

replace k8s.io/controller-manager => k8s.io/controller-manager v0.26.11

replace k8s.io/cri-api => k8s.io/cri-api v0.26.11

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.26.11

replace k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.26.11

replace k8s.io/kms => k8s.io/kms v0.26.11

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.26.11

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.26.11

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.26.11

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.26.11

replace k8s.io/kubectl => k8s.io/kubectl v0.26.11

replace k8s.io/kubelet => k8s.io/kubelet v0.26.11

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.26.11

replace k8s.io/metrics => k8s.io/metrics v0.26.11

replace k8s.io/mount-utils => k8s.io/mount-utils v0.26.11

replace k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.26.11

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.26.11

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.26.11

replace k8s.io/sample-controller => k8s.io/sample-controller v0.26.11
