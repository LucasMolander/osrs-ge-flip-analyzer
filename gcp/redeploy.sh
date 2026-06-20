#!/bin/bash

# Exit on error
set -e

# Anchor execution to the root of the repository
cd "$(dirname "$0")/.."

echo "================================================================="
echo "    OSRS GE Flip Analyzer - Full Build & Redeployment"
echo "================================================================="

# 1. Build Local Binary
echo "Step 1: Compiling local ge-analyzer CLI binary..."
go build -o ge-analyzer ./cmd/ge-analyzer
echo "✅ Local binary compiled successfully."

# Check if env_metadata.sh exists
if [ ! -f ./gcp/env_metadata.sh ]; then
  echo "❌ Error: env_metadata.sh not found!"
  echo "   You must run the initial setup first to create the project:"
  echo "   👉 ./gcp/setup_personal.sh"
  exit 1
fi

# Load shared environment variables
source ./gcp/env_metadata.sh

echo ""
echo "Active Project ID:  ${PROJECT_ID}"
echo "Registry Image:    ${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY_NAME}/${IMAGE_NAME}:latest"
echo "Cloud Run Service: ${SERVICE_NAME} (Limits: ${CPU} CPU, ${MEMORY} Memory)"
echo "================================================================="
echo ""

# Ensure active project is set
echo "Step 2: Setting active gcloud project..."
gcloud config set project "${PROJECT_ID}"

# Configure docker auth
echo ""
echo "Step 3: Authenticating Docker with Artifact Registry..."
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

# Build and Push Image
echo ""
echo "Step 4: Rebuilding container image..."
IMAGE_URL="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY_NAME}/${IMAGE_NAME}:latest"
docker build -t "${IMAGE_URL}" -f "deploy/Dockerfile" .

echo "Pushing new container image to Artifact Registry..."
if ! docker push "${IMAGE_URL}"; then
  echo "⚠️  Failed to push container image. The GCP project or Artifact Registry may not exist anymore."
  echo "   I tried! Exiting..."
  exit 0
fi
echo "✅ Image successfully uploaded."

# Update Cloud Run
echo ""
echo "Step 5: Deploying new image to Cloud Run service..."
if ! gcloud run deploy "${SERVICE_NAME}" \
    --image="${IMAGE_URL}" \
    --region="${REGION}" \
    --port="${CONTAINER_PORT}" \
    --cpu="${CPU}" \
    --memory="${MEMORY}" \
    --project="${PROJECT_ID}" \
    --allow-unauthenticated; then
  echo "⚠️  Failed to deploy to Cloud Run. The service may not be provisioned yet."
  echo "   I tried! Exiting..."
  exit 0
fi

CLOUD_RUN_URL=$(gcloud run services describe "${SERVICE_NAME}" --region="${REGION}" --format="value(status.url)")

echo ""
echo "================================================================="
echo "🎉 Full build and redeployment completed successfully!"
echo "================================================================="
echo "Cloud Run HTTPS URL:  ${CLOUD_RUN_URL}"
echo "================================================================="
