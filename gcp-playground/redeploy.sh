#!/bin/bash

# Exit on error
set -e

# Change directory to the script's folder to make relative paths work correctly
cd "$(dirname "$0")"

echo "================================================================="
echo "    OSRS GE Flip Analyzer - GCP Playground Fast Redeployment"
echo "================================================================="

# Check if env_metadata.sh exists
if [ ! -f ./env_metadata.sh ]; then
  echo "❌ Error: env_metadata.sh not found!"
  echo "   You must run the initial setup first to create the project:"
  echo "   👉 ./setup_playground.sh"
  exit 1
fi

# Load shared environment variables
source ./env_metadata.sh

echo "Active Project ID:  ${PROJECT_ID}"
echo "Registry Image:    ${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY_NAME}/${IMAGE_NAME}:latest"
echo "Cloud Run Service: ${SERVICE_NAME} (Limits: ${CPU} CPU, ${MEMORY} Memory)"
echo "================================================================="
echo ""

# Ensure active project is set
echo "Step 1: Setting active gcloud project..."
gcloud config set project "${PROJECT_ID}"

# Configure docker auth
echo ""
echo "Step 2: Authenticating Docker with Artifact Registry..."
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

# Build and Push Image
echo ""
echo "Step 3: Rebuilding optimized Go binary and container image..."
IMAGE_URL="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY_NAME}/${IMAGE_NAME}:latest"
# Build utilizing the repository root context (..) and root Dockerfile (../Dockerfile)
docker build -t "${IMAGE_URL}" -f "../Dockerfile" ..

echo "Pushing new container image to Artifact Registry..."
docker push "${IMAGE_URL}"
echo "✅ Image successfully uploaded."

# Update Cloud Run
echo ""
echo "Step 4: Deploying new image to Cloud Run service..."
gcloud run deploy "${SERVICE_NAME}" \
    --image="${IMAGE_URL}" \
    --region="${REGION}" \
    --port="${CONTAINER_PORT}" \
    --cpu="${CPU}" \
    --memory="${MEMORY}" \
    --project="${PROJECT_ID}" \
    --no-allow-unauthenticated

CLOUD_RUN_URL=$(gcloud run services describe "${SERVICE_NAME}" --region="${REGION}" --format="value(status.url)")

echo ""
echo "================================================================="
echo "🎉 Redeployment completed successfully!"
echo "================================================================="
echo "Cloud Run HTTPS URL:  ${CLOUD_RUN_URL}"
echo "================================================================="
