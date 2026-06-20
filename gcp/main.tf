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
  project               = var.project_id
  region                = var.region
  user_project_override = true
  billing_project       = var.project_id
  access_token          = var.access_token
}

# --- Variables ---
variable "access_token" {
  description = "OAuth token for Terraform"
  type        = string
  sensitive   = true
}

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

variable "webapp_username" {
  description = "Basic auth username for the dashboard"
  type        = string
}

variable "webapp_password" {
  description = "Basic auth password for the dashboard"
  type        = string
  sensitive   = true
}



# --- Artifact Registry ---
resource "google_artifact_registry_repository" "repo" {
  provider      = google
  location      = var.region
  repository_id = var.repository_name
  format        = "DOCKER"
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

  lifecycle_rule {
    condition {
      age = 7
    }
    action {
      type = "Delete"
    }
  }
}

# --- Cloud Run Service Account ---
resource "google_service_account" "run_sa" {
  account_id   = "ge-analyzer-runner"
  display_name = "Cloud Run Service Account for GE Analyzer"
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
    scaling {
      max_instance_count = 3
    }
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
        name  = "DATA_STORAGE"
        value = "gcs"
      }
      env {
        name  = "GCS_BUCKET"
        value = google_storage_bucket.database.name
      }
      env {
        name  = "AUTH_USERNAME"
        value = var.webapp_username
      }
      env {
        name  = "AUTH_PASSWORD"
        value = var.webapp_password
      }
      env {
        name  = "CRON_SECRET"
        value = random_password.cron_secret.result
      }
    }
  }

  depends_on = [
    google_artifact_registry_repository.repo,
    google_storage_bucket_iam_member.bucket_writer
  ]
}

# --- Cloud Run Access Control ---

# Grant invoker access to the public internet
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  project    = google_cloud_run_v2_service.server.project
  location   = google_cloud_run_v2_service.server.location
  name       = google_cloud_run_v2_service.server.name
  role       = "roles/run.invoker"
  member     = "allUsers"
  depends_on = [google_cloud_run_v2_service.server]
}

resource "random_password" "cron_secret" {
  length  = 32
  special = false
}

# --- Cloud Scheduler ---
resource "google_cloud_scheduler_job" "report_trigger" {
  name             = "${var.service_name}-trigger"
  description      = "Triggers the GE Analyzer Report Job every minute"
  schedule         = "* * * * *" # Every minute
  time_zone        = "UTC"
  region           = var.region

  http_target {
    http_method = "POST"
    uri         = "${google_cloud_run_v2_service.server.uri}/api/internal/cron-tick"

    headers = {
      "X-Cron-Secret" = random_password.cron_secret.result
    }
  }

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
