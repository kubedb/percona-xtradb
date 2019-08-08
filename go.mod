module kubedb.dev/percona-xtradb

go 1.12

require (
	github.com/Azure/azure-pipeline-go v0.1.9 // indirect
	github.com/Azure/azure-storage-blob-go v0.6.0 // indirect
	github.com/appscode/go v0.0.0-20190804161047-d69b20b44118
	github.com/aws/aws-sdk-go v1.20.21 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/census-instrumentation/opencensus-proto v0.2.1 // indirect
	github.com/codeskyblue/go-sh v0.0.0-20190412065543-76bd3d59ff27
	github.com/coreos/bbolt v1.3.3 // indirect
	github.com/coreos/prometheus-operator v0.31.1
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/fatih/structs v1.1.0
	github.com/go-ini/ini v1.25.4 // indirect
	github.com/go-openapi/spec v0.19.2 // indirect
	github.com/go-openapi/swag v0.19.4 // indirect
	github.com/go-sql-driver/mysql v1.4.1
	github.com/go-xorm/xorm v0.7.6
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/google/martian v2.1.1-0.20190517191504-25dcb96d9e51+incompatible // indirect
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/json-iterator/go v1.1.7 // indirect
	github.com/lib/pq v1.1.1 // indirect
	github.com/ncw/swift v1.0.49 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/procfs v0.0.3 // indirect
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	go.etcd.io/bbolt v1.3.3 // indirect
	gomodules.xyz/stow v0.2.0
	google.golang.org/grpc v1.21.2 // indirect
	k8s.io/api v0.0.0-20190503110853-61630f889b3c
	k8s.io/apiextensions-apiserver v0.0.0-20190516231611-bf6753f2aa24
	k8s.io/apimachinery v0.0.0-20190508063446-a3da69d3723c
	k8s.io/apiserver v0.0.0-20190516230822-f89599b3f645
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/klog v0.3.3 // indirect
	k8s.io/kube-aggregator v0.0.0-20190508104018-6d3d96b06d29
	k8s.io/kubernetes v1.14.5 // indirect
	kmodules.xyz/client-go v0.0.0-20190802200916-043217632b6a
	kmodules.xyz/custom-resources v0.0.0-20190802202832-aaad432d3364
	kmodules.xyz/monitoring-agent-api v0.0.0-20190802203207-a87aa5b2e057
	kmodules.xyz/objectstore-api v0.0.0-20190802205146-9816ffafe0d7
	kmodules.xyz/offshoot-api v0.0.0-20190802203449-05938be4a23b
	kmodules.xyz/webhook-runtime v0.0.0-20190802202019-9e77ee949266
	kubedb.dev/apimachinery v0.13.0-rc.0.0.20190808064950-8f78bf3cebde
	stash.appscode.dev/stash v0.9.0-rc.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.3.0+incompatible
	github.com/go-xorm/core v0.6.3 => xorm.io/core v0.6.3
	github.com/graymeta/stow => github.com/appscode/stow v0.0.0-20190506085026-ca5baa008ea3
	gopkg.in/robfig/cron.v2 => github.com/appscode/cron v0.0.0-20170717094345-ca60c6d796d4
	k8s.io/api => k8s.io/api v0.0.0-20190313235455-40a48860b5ab
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed
	k8s.io/apimachinery => github.com/kmodules/apimachinery v0.0.0-20190508045248-a52a97a7a2bf
	k8s.io/apiserver => github.com/kmodules/apiserver v0.0.0-20190508082252-8397d761d4b5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190314001948-2899ed30580f
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190314002645-c892ea32361a
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190314000054-4a91899592f4
	k8s.io/klog => k8s.io/klog v0.3.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190314000639-da8327669ac5
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190228160746-b3a7cee44a30
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190314001731-1bd6a4002213
	k8s.io/utils => k8s.io/utils v0.0.0-20190221042446-c2654d5206da
)
