#!/bin/bash
export GOOS=linux
export GOARCH=amd64
cd bad
go build -o hello-kubernetes
docker build -t gcr.io/$PROJECT_ID/hello-kubernetes:bad .
cd ../good
go build -o hello-kubernetes
docker build -t gcr.io/$PROJECT_ID/hello-kubernetes:good .
cd ..
gcloud docker -- push gcr.io/$PROJECT_ID/hello-kubernetes:good
gcloud docker -- push gcr.io/$PROJECT_ID/hello-kubernetes:bad