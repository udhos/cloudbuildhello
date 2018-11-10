# cloudbuildhello

gsutil mb gs://cloudbuildhello

gsutil ls gs://cloudbuildhello

gcloud builds submit --config cloudbuild.yaml .

