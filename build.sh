#!/bin/bash -x

img_name="function-github-app-get-token"

go build -o function . && {
  rm package/*.xpkg
  go generate ./...
  docker build --push . -t localhost:5001/${img_name}:latest
  crossplane xpkg build -f package --embed-runtime-image=localhost:5001/${img_name}:latest
  crossplane xpkg push -f package/${img_name}-*.xpkg localhost:5001/${img_name}:latest
} && {
  kubectl apply \
    -f example/functions.yaml \
    -f example/composition.yaml \
    -f example/xrd.yaml
}
