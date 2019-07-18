#!/bin/bash
set -xeou pipefail

GOPATH=$(go env GOPATH)
REPO_ROOT="$GOPATH/src/kubedb.dev/percona-xtradb"

export APPSCODE_ENV=prod

pushd $REPO_ROOT

rm -rf dist

./hack/docker/percona-operator/make.sh
./hack/docker/percona-operator/make.sh release

rm dist/.tag

popd
