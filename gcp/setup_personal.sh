#!/bin/bash

# Exit on error
set -e

# Anchor execution to the root of the repository
cd "$(dirname "$0")/.."

# --- Configuration ---
REGION="us-central1"
REPOSITORY_NAME="ge-analyzer-repo"
IMAGE_NAME="ge-analyzer-server"
SERVICE_NAME="ge-analyzer-service"
CONTAINER_PORT="8080"
CPU="2"
MEMORY="1Gi"
if [ -x "/google/data/ro/teams/terraform/bin/terraform" ]; then
  TERRAFORM_CMD="/google/data/ro/teams/terraform/bin/terraform"
elif command -v terraform &> /dev/null; then
  TERRAFORM_CMD="terraform"
else
  TERRAFORM_CMD="/google/bin/releases/g3terraform/runner_main --base_service_dir=. --config_dir=."
fi

echo "================================================================="
echo "   OSRS GE Flip Analyzer - Personal Deployment Bootstrapper"
echo "================================================================="

# --- Credentials Setup ---
CREDS_FILE="gcp/webapp_creds"
if [ -f "$CREDS_FILE" ]; then
  echo "🔑 Found existing webapp credentials in $CREDS_FILE"
  mapfile -t lines < "$CREDS_FILE"
  WEBAPP_USERNAME="${lines[0]}"
  WEBAPP_PASSWORD="${lines[1]}"
else
  echo "🔑 No webapp credentials found. Let's set them up for the dashboard."
  read -p "   Enter desired username: " WEBAPP_USERNAME
  read -s -p "   Enter desired password: " WEBAPP_PASSWORD
  echo ""
  
  # Create the file with restricted permissions
  touch "$CREDS_FILE"
  chmod 600 "$CREDS_FILE"
  echo "$WEBAPP_USERNAME" > "$CREDS_FILE"
  echo "$WEBAPP_PASSWORD" >> "$CREDS_FILE"
  echo "✅ Saved credentials to $CREDS_FILE"
fi
echo "================================================================="

# Check for existing project to avoid billing quota limits
EXISTING_PROJECT=$(gcloud projects list --filter="name:ge-flip-*" --sort-by=~createTime --format="value(projectId)" | head -n 1)

if [ -n "$EXISTING_PROJECT" ]; then
  PROJECT_ID="$EXISTING_PROJECT"
  echo "♻️ Found existing project: $PROJECT_ID. Reusing it to save quota!"
else
  PROJECT_ID="ge-flip-$(date +%s)"
  echo "Project ID:        ${PROJECT_ID}"
  echo "Step 0: Creating Project..."
  gcloud projects create "$PROJECT_ID"
  echo "✅ Project created successfully: $PROJECT_ID"
  echo "   🔗 Pantheon Console: https://console.cloud.google.com/home/dashboard?project=${PROJECT_ID}&authuser=1"
fi

gcloud config set project "$PROJECT_ID"

echo "Checking billing status for $PROJECT_ID..."
BILLING_ENABLED=$(gcloud beta billing projects describe "$PROJECT_ID" --format="value(billingEnabled)" 2>/dev/null || echo "False")

if [ "$BILLING_ENABLED" != "True" ] && [ "$BILLING_ENABLED" != "true" ]; then
  BILLING_ACCOUNT=$(gcloud beta billing accounts list --format="value(name)" | head -n 1)
  if [ -z "$BILLING_ACCOUNT" ]; then
    echo "❌ Error: Could not find a billing account associated with this user."
    exit 1
  fi
  echo "Linking Billing Account: $BILLING_ACCOUNT..."
  gcloud beta billing projects link "$PROJECT_ID" --billing-account="$BILLING_ACCOUNT"
else
  echo "✅ Billing is already enabled for $PROJECT_ID."
fi

echo "================================================================="
echo ""

echo "Step 1: Enabling necessary APIs..."
gcloud services enable \
    serviceusage.googleapis.com \
    cloudresourcemanager.googleapis.com \
    artifactregistry.googleapis.com \
    run.googleapis.com \
    storage.googleapis.com \
    cloudbuild.googleapis.com \
    iamcredentials.googleapis.com \
    iap.googleapis.com \
    cloudscheduler.googleapis.com

echo "Verifying API and IAM propagation (polling for up to 60 seconds)..."
MAX_RETRIES=12
for ((i=1; i<=MAX_RETRIES; i++)); do
  # We test artifact registry list because it verifies both API enablement and IAM permissions
  if gcloud artifacts repositories list --project="$PROJECT_ID" --location="$REGION" &> /dev/null; then
    echo "✅ API and IAM successfully propagated!"
    break
  fi
  
  if [ "$i" -eq "$MAX_RETRIES" ]; then
    echo "⚠️ Warning: Polling timed out. Continuing anyway, but Terraform might fail."
  else
    echo "   Still waiting... ($i/$MAX_RETRIES)"
    sleep 5
  fi
done

# --- 2. Initialize Terraform & Provision Resources ---
echo ""
echo "Step 2: Initializing and running Terraform..."

# Force Terraform to use the exact same identity as the gcloud CLI
OAUTH_TOKEN=$(gcloud auth print-access-token)
export GOOGLE_BILLING_PROJECT="${PROJECT_ID}"

IMAGE_URL="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY_NAME}/${IMAGE_NAME}:latest"

pushd gcp > /dev/null

if [ -f terraform.tfstate ]; then
  if ! grep -q "$PROJECT_ID" terraform.tfstate; then
    echo "⚠️  Stale Terraform state found for a different project. Cleaning up..."
    rm -rf .terraform .terraform.lock.hcl terraform.tfstate terraform.tfstate.backup
  else
    echo "✅ Valid Terraform state found for $PROJECT_ID. Resuming..."
  fi
fi

${TERRAFORM_CMD} init

echo "Provisioning Artifact Registry first..."
${TERRAFORM_CMD} apply -auto-approve -target=google_artifact_registry_repository.repo \
  -var="access_token=${OAUTH_TOKEN}" \
  -var="project_id=${PROJECT_ID}" \
  -var="region=${REGION}" \
  -var="repository_name=${REPOSITORY_NAME}" \
  -var="service_name=${SERVICE_NAME}" \
  -var="image_url=${IMAGE_URL}" \
  -var="webapp_username=${WEBAPP_USERNAME}" \
  -var="webapp_password=${WEBAPP_PASSWORD}" \
  -var="container_port=${CONTAINER_PORT}" \
  -var="cpu=${CPU}" \
  -var="memory=${MEMORY}"

popd > /dev/null
echo "✅ Artifact Registry provisioned."

# --- 3. Build and Push Docker Image ---
echo ""
echo "Step 3: Building and pushing Docker container..."
echo "Configuring docker authentication for Artifact Registry..."
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

# Build utilizing the repository root context (..) and root Dockerfile (../Dockerfile)
echo "Compiling optimized Go binary inside multi-stage Docker build..."
docker build -t "${IMAGE_URL}" -f "deploy/Dockerfile" .

echo "Pushing container image to Artifact Registry..."
docker push "${IMAGE_URL}"
echo "✅ Container successfully uploaded to Artifact Registry."

# --- 4. Deploy and Update Remaining Infrastructure ---
echo ""
echo "Step 4: Provisioning remaining infrastructure (GCS, Cloud Run, IAM bindings)..."
pushd gcp > /dev/null
${TERRAFORM_CMD} apply -auto-approve \
  -var="access_token=${OAUTH_TOKEN}" \
  -var="project_id=${PROJECT_ID}" \
  -var="region=${REGION}" \
  -var="repository_name=${REPOSITORY_NAME}" \
  -var="service_name=${SERVICE_NAME}" \
  -var="image_url=${IMAGE_URL}" \
  -var="webapp_username=${WEBAPP_USERNAME}" \
  -var="webapp_password=${WEBAPP_PASSWORD}" \
  -var="container_port=${CONTAINER_PORT}" \
  -var="cpu=${CPU}" \
  -var="memory=${MEMORY}"

# Fetch the generated bucket name from outputs
GCS_BUCKET_NAME=$(${TERRAFORM_CMD} output -raw gcs_bucket_name)
CLOUD_RUN_URL=$(${TERRAFORM_CMD} output -raw cloud_run_url)
popd > /dev/null

echo "✅ Infrastructure fully provisioned."
echo "   Created persistent GCS storage bucket: gs://${GCS_BUCKET_NAME}"

# --- 5. Output Environment Metadata File ---
echo "Step 5: Outputting env_metadata.sh metadata file..."
cat << METADATA_EOF > gcp/env_metadata.sh
# Generated by setup_personal.sh on $(date)
export PROJECT_ID="${PROJECT_ID}"
export REGION="${REGION}"
export REPOSITORY_NAME="${REPOSITORY_NAME}"
export IMAGE_NAME="${IMAGE_NAME}"
export SERVICE_NAME="${SERVICE_NAME}"
export CONTAINER_PORT="${CONTAINER_PORT}"
export CPU="${CPU}"
export MEMORY="${MEMORY}"
export GCS_BUCKET_NAME="${GCS_BUCKET_NAME}"
METADATA_EOF
chmod +x gcp/env_metadata.sh
echo "✅ Shared environment metadata file created: env_metadata.sh"

echo ""
echo "================================================================="
echo "🎉 Personal deployment bootstrapper finished successfully!"
echo "================================================================="
echo "Project ID:            ${PROJECT_ID}"
echo "Persistent GCS Bucket: gs://${GCS_BUCKET_NAME}"
echo "Cloud Run HTTPS URL:   ${CLOUD_RUN_URL}"
echo "================================================================="
echo ""
echo "👉 You can now access your dashboard securely from any browser!"
echo "   URL: ${CLOUD_RUN_URL}"
echo "   Login: ${WEBAPP_USERNAME} / [hidden]"
echo "================================================================="
