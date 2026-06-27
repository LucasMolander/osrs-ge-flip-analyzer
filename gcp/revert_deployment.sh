#!/bin/bash

# Exit on error
set -e

# Anchor execution to the root of the repository
cd "$(dirname "$0")/.."

echo "================================================================="
echo "    OSRS GE Flip Analyzer - Revert Deployment"
echo "================================================================="

# Check if env_metadata.sh exists
if [ ! -f ./gcp/env_metadata.sh ]; then
  echo "❌ Error: env_metadata.sh not found!"
  exit 1
fi

# Load shared environment variables
source ./gcp/env_metadata.sh

echo "Active Project ID:  ${PROJECT_ID}"
echo "Cloud Run Service: ${SERVICE_NAME}"
echo "Region:            ${REGION}"
echo "================================================================="
echo ""

# Ensure active project is set
echo "Step 1: Setting active gcloud project..."
gcloud config set project "${PROJECT_ID}" --quiet

echo ""
echo "Step 2: Finding the most recent successful revision..."
# We look for a revision that has a Ready condition of True, sorting by creationTimestamp descending
LATEST_SUCCESSFUL=$(gcloud run revisions list \
  --service "${SERVICE_NAME}" \
  --region "${REGION}" \
  --filter="status.conditions.type=Ready AND status.conditions.status=True" \
  --sort-by="~metadata.creationTimestamp" \
  --limit=1 \
  --format="value(metadata.name)")

if [ -z "${LATEST_SUCCESSFUL}" ]; then
  echo "❌ Error: Could not find any successful (Ready=True) revisions for this service."
  exit 1
fi

echo "✅ Found latest successful revision: ${LATEST_SUCCESSFUL}"

echo ""
echo "Step 3: Routing 100% of traffic to ${LATEST_SUCCESSFUL}..."
if gcloud run services update-traffic "${SERVICE_NAME}" \
  --region="${REGION}" \
  --to-revisions="${LATEST_SUCCESSFUL}=100" \
  --quiet; then
  echo "✅ Traffic successfully routed."
else
  echo "❌ Failed to route traffic."
  exit 1
fi

echo ""
echo "================================================================="
echo "🎉 Revert completed successfully! The service is now using ${LATEST_SUCCESSFUL}."
echo "================================================================="
