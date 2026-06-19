terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# --- Variables ---
variable "project_id" {
  description = "The GCP project ID"
  type        = string
}

variable "region" {
  description = "The GCP region"
  type        = string
}

variable "repository_name" {
  description = "Artifact Registry repository name"
  type        = string
}

variable "service_name" {
  description = "Cloud Run service name"
  type        = string
}

variable "image_url" {
  description = "Full URL of the container image"
  type        = string
}

variable "container_port" {
  description = "Port the container listens on"
  type        = number
}

variable "cpu" {
  description = "Cloud Run CPU allocation"
  type        = string
}

variable "memory" {
  description = "Cloud Run Memory allocation"
  type        = string
}

variable "corp_email" {
  description = "Your Corp Email"
  type        = string
}

variable "personal_email" {
  description = "Your Personal Gmail for IAP Access"
  type        = string
}

# --- APIs to Enable ---
resource "google_project_service" "apis" {
  for_each = toset([
    "artifactregistry.googleapis.com",
    "run.googleapis.com",
    "storage.googleapis.com",
    "iap.googleapis.com",
    "cloudbuild.googleapis.com",
    "iamcredentials.googleapis.com",
  ])
  service            = each.key
  disable_on_destroy = false
}

# --- Artifact Registry ---
resource "google_artifact_registry_repository" "repo" {
  provider      = google
  location      = var.region
  repository_id = var.repository_name
  format        = "DOCKER"
  depends_on    = [google_project_service.apis]
}

# --- Persistent GCS Bucket ---
resource "random_id" "bucket_suffix" {
  byte_length = 4
}

resource "google_storage_bucket" "database" {
  name                        = "${var.project_id}-data-${random_id.bucket_suffix.hex}"
  location                    = var.region
  force_destroy               = true
  uniform_bucket_level_access = true
  depends_on                  = [google_project_service.apis]
}

# --- Cloud Run Service Account ---
resource "google_service_account" "run_sa" {
  account_id   = "ge-analyzer-runner"
  display_name = "Cloud Run Service Account for GE Analyzer"
  depends_on   = [google_project_service.apis]
}

resource "google_storage_bucket_iam_member" "bucket_writer" {
  bucket     = google_storage_bucket.database.name
  role       = "roles/storage.objectAdmin"
  member     = "serviceAccount:${google_service_account.run_sa.email}"
  depends_on = [google_storage_bucket.database, google_service_account.run_sa]
}

# --- Cloud Run V2 Service ---
resource "google_cloud_run_v2_service" "server" {
  provider     = google
  name         = var.service_name
  location     = var.region
  ingress      = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.run_sa.email
    containers {
      image = var.image_url
      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }
      ports {
        container_port = var.container_port
      }
      env {
        name  = "GCS_BUCKET"
        value = google_storage_bucket.database.name
      }
    }
  }
  depends_on = [
    google_artifact_registry_repository.repo,
    google_storage_bucket_iam_member.bucket_writer
  ]
}

# --- Cloud Run Access Control ---

# Grant invoker access ONLY to your corp account
resource "google_cloud_run_v2_service_iam_member" "corp_invoker" {
  project    = google_cloud_run_v2_service.server.project
  location   = google_cloud_run_v2_service.server.location
  name       = google_cloud_run_v2_service.server.name
  role       = "roles/run.invoker"
  member     = "user:${var.corp_email}"
  depends_on = [google_cloud_run_v2_service.server]
}

# Grant invoker access ONLY to your personal account
resource "google_cloud_run_v2_service_iam_member" "personal_invoker" {
  project    = google_cloud_run_v2_service.server.project
  location   = google_cloud_run_v2_service.server.location
  name       = google_cloud_run_v2_service.server.name
  role       = "roles/run.invoker"
  member     = "user:${var.personal_email}"
  depends_on = [google_cloud_run_v2_service.server]
}

# --- Outputs ---
output "cloud_run_url" {
  value = google_cloud_run_v2_service.server.uri
}

output "project_id" {
  value = var.project_id
}

output "gcs_bucket_name" {
  value = google_storage_bucket.database.name
}
