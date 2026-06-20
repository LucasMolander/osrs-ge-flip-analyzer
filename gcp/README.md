# `gcp`

This directory contains the Infrastructure as Code (IaC) and automation scripts required to deploy the OSRS GE Flip Analyzer to Google Cloud Platform (GCP). The application runs as a Docker container on Google Cloud Run.

## Files

- **`main.tf`**: The Terraform configuration file. It provisions necessary GCP infrastructure, including the Artifact Registry repository, the Cloud Run service, and the associated IAM permissions required for public HTTP invocation. It restricts concurrent instances to avoid overriding data.
- **`setup_personal.sh`**: An initialization script designed to bootstrap a fresh deployment. It enables required GCP APIs, builds the Docker image locally via Cloud Build, runs `terraform apply`, and handles the initial push to Cloud Run.
- **`redeploy.sh`**: A fast deployment script used to push updates. It packages the source code, builds the container image using `gcloud builds submit`, and seamlessly patches the existing Cloud Run service with the new image.
- **`env_metadata.sh`**: Helper script used to cache or verify the project ID and GCP environment context to prevent accidental deployments to the wrong project.
