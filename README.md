# RSS Alert + SMTP (Go)

Fetches a phpBB Atom feed, deduplicates posts via Firestore, and sends a single HTML email containing every new entry.

## What it does
1. Downloads a configured Atom feed URL.
2. Tracks “seen” entry IDs inside Firestore’s `seen_entries` collection.
3. Batches any brand-new posts into a single HTML message.
4. Sends the message through the SMTP relay you configure.

## Prerequisites
- Go 1.26 or newer.
- A Google Cloud project with Firestore enabled and the deployed service account granted Firestore read/write permissions.
- A working SMTP relay (Zoho, SendGrid, etc.) whose credentials you can store in environment variables.

## Environment
| Name | Description |
| --- | --- |
| `FEED_URL` | Atom feed URL to poll (for example, the RedFlagDeals topic feed). |
| `SMTP_HOST` | SMTP host (e.g., `smtp.zohocloud.ca`). |
| `SMTP_USER` | SMTP username (typically the sender email). |
| `SMTP_PASS` | SMTP password or app-specific secret. |
| `GOOGLE_CLOUD_PROJECT` / `GCP_PROJECT` | GCP project ID for Firestore initialization. |
| `FIREBASE_DATABASE_ID` | (Optional) Firestore database ID; defaults to Firestore’s `(default)` database. |

## Local development
1. Install Go and make sure `C:\Program Files\Go\bin` is on your PATH.
2. Set the required environment variables:
   ```powershell
   $env:FEED_URL="https://forums.redflagdeals.com/feed/..."
   $env:SMTP_HOST="smtp.zohocloud.ca"
   $env:SMTP_USER="you@example.com"
   $env:SMTP_PASS="secret"
   $env:GOOGLE_CLOUD_PROJECT="your-project-id"
   ```
3. Run the service:
   ```
   go run main.go
   ```

## Deploy to Cloud Run
Cloud Run builds the Go binary and downloads Firestore dependencies automatically:
```
gcloud run deploy rss-alert \
  --source . \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars FEED_URL=<feed>,SMTP_HOST=<host>,SMTP_USER=<user>,SMTP_PASS=<pass>,GOOGLE_CLOUD_PROJECT=<project>
```
- Make sure the Cloud Run service account has the Firestore role it needs (Firestore User or Firestore Client).
- If you need scheduling, trigger the service via Cloud Scheduler or add an internal ticker loop.

## Continuous deploy with Cloud Build
Use the provided `cloudbuild.yaml` to build, push, and deploy on every commit:

1. `go test ./...` to verify the code.
2. Build/push `gcr.io/$PROJECT_ID/rss-alert` using the `Dockerfile` in this repo.
3. Run `gcloud run deploy` with environment-variable substitutions for `_FEED_URL`, `_SMTP_HOST`, `_SMTP_USER`, `_SMTP_PASS`, `_REGION`, and `_FIREBASE_DATABASE_ID`.
4. Provide `_FIREBASE_DATABASE_ID` only if you need to use a non-default Firestore database.

Create a Cloud Build trigger that overrides those substitutions with your production values. For secrets (SMTP password) use Cloud Build Secrets or Secret Manager so credentials never land in source control.

## Firestore layout
- Collection: `seen_entries`
- Document ID: Atom entry `ID` (unique per post).
- Fields: `seenAt` (server timestamp added when the entry is marked).

## Testing
- Point the code at a test Firestore dataset or local emulator and run `go run main.go` with the same env vars to verify the fetch/send loop.
