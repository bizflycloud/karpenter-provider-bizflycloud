#!/bin/bash
set -e

# Default tag
TAG=${TAG:-latest}
REGISTRY="cr-hn-1.bizflycloud.vn/31ff9581861a4d0ea4df5e7dda0f665d"
IMAGE_NAME="karpenter-provider-bizflycloud"
FULL_IMAGE_NAME="${REGISTRY}/${IMAGE_NAME}:${TAG}"

echo "Building Docker image: ${FULL_IMAGE_NAME}"

# Build the Docker image
docker build -t ${FULL_IMAGE_NAME} .

# Push the Docker image
echo "Pushing Docker image: ${FULL_IMAGE_NAME}"
docker push ${FULL_IMAGE_NAME}

echo "Done! Image pushed to ${FULL_IMAGE_NAME}"