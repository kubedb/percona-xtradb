#!/bin/bash
set -xeou pipefail

GOPATH=$(go env GOPATH)
REPO_ROOT=$GOPATH/src/github.com/kubedb/percona

source "$REPO_ROOT/hack/libbuild/common/lib.sh"
source "$REPO_ROOT/hack/libbuild/common/kubedb_image.sh"

DOCKER_REGISTRY=${DOCKER_REGISTRY:-kubedb}

IMG="percona-xtradb-cluster"

DB_VERSION="5.7"
TAG="$DB_VERSION"

build() {
  pushd "$REPO_ROOT/hack/docker/percona/$DB_VERSION"

  # Download Peer-finder
  # ref: peer-finder: https://github.com/kmodules/peer-finder/releases/download/v1.0.1-ac/peer-finder
  # wget peer-finder: https://github.com/kubernetes/charts/blob/master/stable/mongodb-replicaset/install/Dockerfile#L18
  wget -qO peer-finder https://github.com/kmodules/peer-finder/releases/download/v1.0.1-ac/peer-finder
  chmod +x peer-finder
  chmod +x on-start.sh

  local cmd="docker build --pull -t $DOCKER_REGISTRY/$IMG:$TAG ."
  echo $cmd
  $cmd

  rm peer-finder
  popd
}

binary_repo $@

#docker pull percona/$IMG:$DB_VERSION
#
#docker tag percona/$IMG:$DB_VERSION $DOCKER_REGISTRY/$IMG:$TAG
#docker push $DOCKER_REGISTRY/$IMG:$TAG
